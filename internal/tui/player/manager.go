package player

import (
	"context"
	"fmt"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/LukeHagar/plexgo/models/operations"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/termgraphics"
)

type PlayerManager struct {
	// ioMu serializes commands and lifecycle changes. Rendering only takes
	// statusMu, so a slow IPC request never blocks the terminal UI.
	ioMu           sync.Mutex
	conn           *ipcClient
	overlay        atomic.Pointer[termgraphics.OverlayWriter]
	redrawOverlay  atomic.Bool
	overlayProcess *os.Process
	output         config.VideoOutput
	queue          []queueEntry // guarded by ioMu; mpv playlist IDs survive track changes
	cancel         context.CancelFunc
	socketPath     string
	waveformDir    string
	statusMu       sync.RWMutex
	status         PlayerStatus
	updates        chan tea.Msg
	done           chan struct{}
	closeOnce      sync.Once
	workers        sync.WaitGroup
	requestMu      sync.Mutex
	requestCancel  context.CancelFunc
	reportOnce     sync.Once
	reports        chan reportBatch
	reporterDone   chan struct{}
	reportCtx      context.Context
	reportCancel   context.CancelFunc
}

var (
	instance *PlayerManager
	once     sync.Once
)

func GetPlayerManager() *PlayerManager {
	once.Do(func() {
		socket := filepath.Join(config.Get().CacheDir, "mpv.sock")
		if runtime.GOOS == "windows" {
			socket = "\\\\.\\pipe\\mpv-socket"
		} else {
			_ = os.MkdirAll(filepath.Dir(socket), 0o755)
		}
		instance = newPlayerManager(socket)
	})
	return instance
}

func newPlayerManager(socket string) *PlayerManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &PlayerManager{socketPath: socket, updates: make(chan tea.Msg, 1), done: make(chan struct{}),
		reports: make(chan reportBatch, 64), reportCtx: ctx, reportCancel: cancel}
}

// Register selections when the command is created, before Bubble Tea schedules
// it. Slow metadata requests must never replace a newer selection or a stop.
func (pm *PlayerManager) beginRequest(parent context.Context) context.Context {
	pm.requestMu.Lock()
	defer pm.requestMu.Unlock()
	if pm.requestCancel != nil {
		pm.requestCancel()
	}
	// This context also owns foreground playback, which may last for hours.
	// Metadata resolution and startup impose their own bounded deadlines.
	ctx, cancel := context.WithCancel(parent)
	pm.requestCancel = cancel
	select {
	case <-pm.done:
		cancel()
	default:
	}
	return ctx
}

func (pm *PlayerManager) Status() PlayerStatus {
	pm.statusMu.RLock()
	defer pm.statusMu.RUnlock()
	return pm.status
}

func (pm *PlayerManager) setStatus(status PlayerStatus) {
	pm.statusMu.Lock()
	pm.status = status
	pm.statusMu.Unlock()
}

func (pm *PlayerManager) VerifyConnection() bool {
	pm.ioMu.Lock()
	defer pm.ioMu.Unlock()
	return pm.conn != nil && pm.conn.get("mpv-version", nil) == nil
}

func (pm *PlayerManager) Play(
	url, title, ratingKey string, noReport bool, output config.VideoOutput,
	startOffset int64, subtitles []ExternalSubtitle, servers ...config.ServerTarget,
) tea.Cmd {
	return pm.PlayRequest(&PlayRequest{
		Items:  []PlayItem{{URL: url, Title: title, RatingKey: ratingKey}},
		Server: config.MediaServer(servers...), NoReport: noReport, Subtitles: subtitles,
	}, output, startOffset)
}

func (pm *PlayerManager) PlayRequest(req *PlayRequest, output config.VideoOutput, startOffset int64) tea.Cmd {
	return pm.playRequest(pm.beginRequest(context.Background()), req, output, startOffset)
}

func (pm *PlayerManager) playRequest(ctx context.Context, req *PlayRequest, output config.VideoOutput, startOffset int64) tea.Cmd {
	if req == nil || len(req.Items) == 0 {
		return func() tea.Msg { return fmt.Errorf("empty playback queue") }
	}
	if isOverlayOutput(output) && (!config.IsTUI || config.Get().TerminalVideoMode != "overlay") {
		return pm.playTerminalRequest(ctx, req, output, startOffset)
	}
	preference := output
	return func() tea.Msg {
		if isOverlayOutput(output) {
			_, resolved, err := terminalOutput(ctx, output, os.Getenv)
			if err != nil {
				return err
			}
			output = resolved
		}
		_, err := pm.startRequest(ctx, req, output, startOffset)
		if err != nil && ctx.Err() == nil && preference == config.VideoAuto && output != config.VideoTCT {
			if _, _, probeErr := terminalOutput(ctx, config.VideoTCT, os.Getenv); probeErr == nil {
				if _, retryErr := pm.startRequest(ctx, req, config.VideoTCT, startOffset); retryErr == nil {
					pm.sendUpdate()
					return VideoFallbackMsg{From: output}
				}
			}
		}
		if err != nil {
			if ctx.Err() == context.Canceled {
				return nil
			}
			return err
		}
		pm.sendUpdate()
		return nil
	}
}

func (pm *PlayerManager) startRequest(ctx context.Context, req *PlayRequest, output config.VideoOutput, startOffset int64) (PlayerStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	pm.ioMu.Lock()
	defer pm.ioMu.Unlock()
	previous := pm.Status()
	if err := ctx.Err(); err != nil {
		return previous, err
	}
	select {
	case <-pm.done:
		return previous, fmt.Errorf("music player is closed")
	default:
	}
	if pm.conn != nil {
		_ = pm.conn.get("idle-active", nil)
		pm.enqueueReports(pm.queueEvents(&previous))
		pm.setStatus(previous)
	}
	if err := pm.connect(output); err != nil {
		return previous, err
	}
	failed := true
	defer func() {
		if failed {
			_ = pm.conn.call("stop")
			pm.finishPrevious(&previous)
			pm.queue = nil
			pm.setStatus(PlayerStatus{})
		}
	}()
	audioOnly := output == config.AudioTerminal
	_ = pm.conn.call("af", "remove", "@plexctl-spectrum")
	if audioOnly {
		_ = pm.conn.call("af", "add", spectrumFilter())
		_ = pm.ensureWaveformScript()
	}
	// Pause while constructing the playlist. Boundary events are also buffered
	// because a decoder can finish a tiny file even before audio is unpaused.
	for i, item := range req.Items {
		if err := ctx.Err(); err != nil {
			return previous, err
		}
		offset := int64(0)
		if i == 0 {
			offset = startOffset
		}
		command := mediaLoadCommand(item.URL, offset, audioOnly)
		options := command["options"].(map[string]string)
		options["lavfi-complex"] = ""
		if isOverlayOutput(output) {
			options["vo"], options["force-window"] = string(output), "no"
		}
		if req.Music && !audioOnly {
			options["lavfi-complex"] = musicVideoGraph(config.Get().WindowMusicVisualizer)
		}
		options["force-media-title"] = item.Title
		options["pause"] = "yes"
		if i > 0 {
			command["flags"] = "append"
			options["pause"] = "no"
		}
		if err := pm.conn.command(command, nil); err != nil {
			return previous, err
		}
	}
	var playlist []struct {
		ID int64 `json:"id"`
	}
	if err := pm.conn.get("playlist", &playlist); err != nil || len(playlist) != len(req.Items) {
		return previous, fmt.Errorf("mpv did not accept the complete playback queue")
	}
	expectVideo := isOverlayOutput(output) && (!req.Music || musicVideoGraph(config.Get().WindowMusicVisualizer) != "")
	if err := pm.conn.waitInitialFile(ctx, expectVideo); err != nil {
		return previous, err
	}
	pm.finishPrevious(&previous)
	pm.queue = make([]queueEntry, len(req.Items))
	for i, item := range req.Items {
		pm.queue[i] = queueEntry{ID: playlist[i].ID, PlayItem: item}
	}
	_ = pm.conn.call("set_property", "user-data/plex-queue", pm.queue)
	_ = pm.conn.call("set_property", "user-data/plex-server-id", req.Server.ID)
	_ = pm.conn.call("set_property", "user-data/plex-server-url", req.Server.URL)
	_ = pm.conn.call("set_property", "user-data/plex-no-report", req.NoReport)
	_ = pm.conn.call("set_property", "user-data/plex-audio-only", audioOnly)
	for _, sub := range req.Subtitles {
		_ = pm.conn.call("sub-add", sub.URL, "auto", sub.Title, sub.Language)
	}
	status := PlayerStatus{Server: req.Server, NoReport: req.NoReport, AudioOnly: audioOnly, Overlay: isOverlayOutput(output),
		State: operations.StatePlaying, Volume: previous.Volume}
	setTrack(&status, req.Items[0])
	status.Time = float64(startOffset) / 1000
	pm.setStatus(status)
	pm.persistTrack(status)
	// Even a paused decoder can reach EOF for a very short file. Retain new
	// playlist boundaries buffered while building or initializing the queue.
	_ = pm.conn.call("set_property", "pause", false)
	pm.enqueueReports([]PlayerStatus{status})
	failed = false
	return previous, nil
}

// Read old playlist boundaries before replacing their ID mapping. Completion
// can occur between the monitor tick and the user's next selection.
func (pm *PlayerManager) finishPrevious(status *PlayerStatus) {
	boundaries := pm.consumeQueueEvents(status, true)
	if status.Active() {
		status.State = operations.StateStopped
		boundaries = append(boundaries, *status)
	}
	pm.enqueueReports(boundaries)
}

type queueEntry struct {
	ID int64
	PlayItem
}

func setTrack(status *PlayerStatus, item PlayItem) {
	status.File, status.Key, status.Title = item.URL, item.RatingKey, item.Title
	status.Time, status.Duration = 0, item.Duration
	status.Spectrum, status.Waveform = Spectrum{}, Waveform{}
}

func (pm *PlayerManager) persistTrack(status PlayerStatus) {
	_ = pm.conn.call("set_property", "force-media-title", status.Title)
	_ = pm.conn.call("set_property", "user-data/plex-rating-key", status.Key)
	_ = pm.conn.call("set_property", "user-data/plex-title", status.Title)
}

// queueEvents consumes actual mpv file boundaries rather than inferring EOF
// from polling. Even tracks shorter than the monitor interval get a final
// timeline report. Called only while ioMu is held.
func (pm *PlayerManager) queueEvents(status *PlayerStatus) []PlayerStatus {
	return pm.consumeQueueEvents(status, false)
}

func (pm *PlayerManager) consumeQueueEvents(status *PlayerStatus, preserveUnknown bool) []PlayerStatus {
	reports := []PlayerStatus{}
	events := pm.conn.events
	pm.conn.events = nil
	for _, event := range events {
		var entry *queueEntry
		for i := range pm.queue {
			if pm.queue[i].ID == event.PlaylistEntryID {
				entry = &pm.queue[i]
				break
			}
		}
		if entry == nil {
			if preserveUnknown && event.Event != "file-loaded" {
				pm.conn.events = append(pm.conn.events, event)
			}
			continue
		}
		switch event.Event {
		case "start-file":
			setTrack(status, entry.PlayItem)
			status.Play()
			reports = append(reports, *status)
		case "end-file":
			ended := *status
			if ended.Key != entry.RatingKey || ended.File != entry.URL {
				setTrack(&ended, entry.PlayItem)
			}
			if event.Reason == "eof" && ended.Duration > 0 {
				ended.Time = ended.Duration
			}
			ended.State = operations.StateStopped
			reports = append(reports, ended)
			*status = ended
		}
	}
	return reports
}

// connect is called with ioMu held. Connection identity prevents a stale
// monitor from clearing a replacement player.
func (pm *PlayerManager) connect(output config.VideoOutput) error {
	if pm.conn != nil && output != pm.output && (isOverlayOutput(output) || isOverlayOutput(pm.output)) {
		_ = pm.conn.call("quit")
		pm.disconnect()
	}
	pm.output = output
	if pm.conn != nil && pm.conn.get("mpv-version", nil) == nil {
		return nil
	}
	pm.disconnect()
	if conn, err := openIPC(pm.socketPath); err == nil {
		if conn.get("mpv-version", nil) == nil {
			if !isOverlayOutput(output) {
				pm.attach(conn)
				return nil
			}
			_ = conn.call("quit")
		}
		_ = conn.conn.Close()
	}
	mpv, err := exec.LookPath("mpv")
	if err != nil {
		return fmt.Errorf("mpv command not found; please install mpv: %w", err)
	}
	if runtime.GOOS != "windows" {
		_ = os.Remove(pm.socketPath)
	}
	// Embedded album art must never create a window for in-terminal music.
	cmd := exec.Command(mpv, "--idle=yes", "--no-resume-playback", "--force-window=no",
		"--terminal=no", "--input-terminal=no", "--input-ipc-server="+pm.socketPath)
	var overlay *termgraphics.OverlayWriter
	if isOverlayOutput(output) {
		overlay = pm.configureOverlay(cmd, output)
	}
	if err := cmd.Start(); err != nil {
		if overlay != nil {
			_ = overlay.Close()
		}
		return fmt.Errorf("start mpv: %w", err)
	}
	if overlay != nil {
		pm.overlay.Store(overlay)
		pm.overlayProcess = cmd.Process
	}
	exited := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		if overlay != nil {
			_ = overlay.Close()
		}
		exited <- err
	}()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	retry := time.NewTicker(50 * time.Millisecond)
	defer retry.Stop()
	for {
		select {
		case <-exited:
			return fmt.Errorf("mpv exited before opening its playback connection")
		case <-deadline.C:
			_ = cmd.Process.Kill()
			<-exited
			return fmt.Errorf("timed out connecting to mpv")
		case <-retry.C:
			conn, err := openIPC(pm.socketPath)
			if err != nil {
				continue
			}
			if conn.get("mpv-version", nil) == nil {
				pm.attach(conn)
				return nil
			}
			_ = conn.conn.Close()
		}
	}
}

func (pm *PlayerManager) attach(conn *ipcClient) {
	ctx, cancel := context.WithCancel(context.Background())
	pm.conn, pm.cancel = conn, cancel
	pm.ensureReporter()
	pm.workers.Add(1)
	go func() {
		defer pm.workers.Done()
		pm.monitor(ctx, conn)
	}()
}

// One reporter survives connection replacements. All producers enqueue while
// holding ioMu, so a delayed "playing" request cannot overtake a later "stopped"
// report or the next track's start. HTTP never holds the playback mutex.
func (pm *PlayerManager) ensureReporter() {
	pm.reportOnce.Do(func() {
		pm.reporterDone = make(chan struct{})
		go func() {
			defer close(pm.reporterDone)
			for batch := range pm.reports {
				for _, status := range batch.statuses {
					if pm.reportCtx.Err() == nil {
						reportProgress(pm.reportCtx, status)
					}
				}
				if batch.done != nil {
					close(batch.done)
				}
			}
		}()
	})
}

type reportBatch struct {
	statuses []PlayerStatus
	done     chan struct{}
}

func (pm *PlayerManager) enqueueReports(batch []PlayerStatus) {
	if len(batch) == 0 {
		return
	}
	select {
	case pm.reports <- reportBatch{statuses: batch}:
	case <-pm.reportCtx.Done():
	}
}

func (pm *PlayerManager) reportTerminal(_ context.Context, status PlayerStatus) {
	pm.ioMu.Lock()
	defer pm.ioMu.Unlock()
	select {
	case <-pm.done:
		return
	default:
	}
	pm.ensureReporter()
	pm.enqueueReports([]PlayerStatus{status})
}

func (pm *PlayerManager) flushReports() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan struct{})
	pm.ioMu.Lock()
	select {
	case <-pm.done:
		pm.ioMu.Unlock()
		return
	default:
	}
	pm.ensureReporter()
	select {
	case pm.reports <- reportBatch{done: done}:
	case <-ctx.Done():
	}
	pm.ioMu.Unlock()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (pm *PlayerManager) Reconnect() tea.Cmd {
	return func() tea.Msg {
		pm.ioMu.Lock()
		defer pm.ioMu.Unlock()
		select {
		case <-pm.done:
			return nil
		default:
		}
		if pm.conn != nil {
			return nil
		}
		conn, err := openIPC(pm.socketPath)
		if err != nil {
			return nil
		}
		if conn.get("mpv-version", nil) != nil {
			_ = conn.conn.Close()
			return nil
		}
		var status PlayerStatus
		_ = conn.get("user-data/plex-queue", &pm.queue)
		_ = conn.get("user-data/plex-title", &status.Title)
		_ = conn.get("user-data/plex-rating-key", &status.Key)
		_ = conn.get("user-data/plex-no-report", &status.NoReport)
		_ = conn.get("user-data/plex-audio-only", &status.AudioOnly)
		_ = conn.get("path", &status.File)
		_ = conn.get("user-data/plex-server-id", &status.Server.ID)
		_ = conn.get("user-data/plex-server-url", &status.Server.URL)
		if mediaURL, err := neturl.Parse(status.File); err == nil {
			status.Server.Token = mediaURL.Query().Get("X-Plex-Token")
		}
		if status.Server.ID == "" || status.Server.URL == "" || status.Server.Token == "" {
			status.NoReport = true // An older player cannot identify its originating server.
		}
		var playlist []struct {
			ID      int64 `json:"id"`
			Current bool  `json:"current"`
		}
		_ = conn.get("playlist", &playlist)
		currentID := int64(-1)
		for _, item := range playlist {
			if item.Current {
				currentID = item.ID
			}
		}
		for _, entry := range pm.queue {
			if entry.ID == currentID {
				setTrack(&status, entry.PlayItem)
				break
			}
		}
		pm.setStatus(status)
		pm.attach(conn)
		return nil
	}
}

func (pm *PlayerManager) control(args ...any) tea.Cmd {
	return func() tea.Msg {
		pm.ioMu.Lock()
		defer pm.ioMu.Unlock()
		if pm.conn == nil {
			return nil
		}
		if err := pm.conn.call(args...); err != nil {
			return fmt.Errorf("player control failed: %w", err)
		}
		return nil
	}
}

func (pm *PlayerManager) TogglePause() tea.Cmd { return pm.control("cycle", "pause") }
func (pm *PlayerManager) Seek(seconds float64) tea.Cmd {
	return pm.control("seek", seconds, "relative")
}
func (pm *PlayerManager) SeekPercent(percent float64) tea.Cmd {
	return pm.control("seek", max(0, min(100, percent)), "absolute-percent")
}
func (pm *PlayerManager) Volume(delta float64) tea.Cmd { return pm.control("add", "volume", delta) }
func (pm *PlayerManager) ToggleMute() tea.Cmd          { return pm.control("cycle", "mute") }

func (pm *PlayerManager) StopPlayback() tea.Cmd {
	ctx := pm.beginRequest(context.Background())
	return func() tea.Msg { return pm.stopCurrent(ctx) }
}

func (pm *PlayerManager) stopCurrent(ctx context.Context) tea.Msg {
	pm.ioMu.Lock()
	select {
	case <-pm.done:
		pm.ioMu.Unlock()
		return nil
	default:
	}
	if ctx.Err() != nil {
		pm.ioMu.Unlock()
		return nil
	}
	previous := pm.Status()
	boundaries := []PlayerStatus{}
	if pm.conn != nil {
		_ = pm.conn.call("quit")
		boundaries = pm.queueEvents(&previous)
	}
	if previous.Active() {
		previous.State = operations.StateStopped
		boundaries = append(boundaries, previous)
	}
	if len(boundaries) > 0 && pm.reporterDone != nil {
		pm.enqueueReports(boundaries)
	}
	pm.disconnect()
	pm.setStatus(PlayerStatus{})
	pm.ioMu.Unlock()
	pm.sendUpdate()
	return nil
}

func (pm *PlayerManager) Stop() tea.Cmd {
	return func() tea.Msg { pm.StopPlayback()(); return tea.Quit() }
}

// Close stops in-terminal music and releases IPC. An external player the user
// elected to keep running can continue after the TUI exits.
func (pm *PlayerManager) Close() {
	pm.closeOnce.Do(func() {
		// Start the bound before waiting for a producer that may be blocked on
		// reporting backpressure while holding ioMu.
		timer := time.AfterFunc(3*time.Second, pm.reportCancel)
		defer timer.Stop()
		defer pm.reportCancel()
		pm.requestMu.Lock()
		if pm.requestCancel != nil {
			pm.requestCancel()
		}
		pm.requestMu.Unlock()
		pm.ioMu.Lock()
		status := pm.Status()
		if (status.AudioOnly || status.Overlay) && pm.conn != nil {
			_ = pm.conn.call("quit")
			boundaries := pm.queueEvents(&status)
			if status.Active() {
				status.State = operations.StateStopped
				boundaries = append(boundaries, status)
			}
			if len(boundaries) > 0 {
				pm.enqueueReports(boundaries)
			}
		}
		close(pm.done)
		pm.disconnect()
		pm.setStatus(PlayerStatus{})
		pm.ioMu.Unlock()
		pm.workers.Wait()
		if pm.reporterDone != nil {
			close(pm.reports)
			<-pm.reporterDone
		}
		if pm.waveformDir != "" {
			_ = os.RemoveAll(pm.waveformDir)
		}
	})
}

func (pm *PlayerManager) disconnect() {
	if overlay := pm.overlay.Swap(nil); overlay != nil {
		_ = overlay.Close()
	}
	if pm.overlayProcess != nil {
		_ = pm.overlayProcess.Kill()
		pm.overlayProcess = nil
	}
	if pm.cancel != nil {
		pm.cancel()
		pm.cancel = nil
	}
	pm.queue = nil
	if pm.conn != nil {
		_ = pm.conn.conn.Close()
		pm.conn = nil
	}
}

func (pm *PlayerManager) monitor(ctx context.Context, conn *ipcClient) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	lastReport := time.Now()
	lastRedraw := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pm.ioMu.Lock()
			if ctx.Err() != nil || pm.conn != conn {
				pm.ioMu.Unlock()
				return
			}
			previous := pm.Status()
			status := previous
			var idle, paused bool
			err := conn.get("idle-active", &idle)
			boundaries := pm.queueEvents(&status)
			sample := status
			if err == nil && !idle {
				_ = conn.get("pause", &paused)
				sample.State = operations.StatePlaying
				if paused {
					sample.State = operations.StatePaused
				}
				_ = conn.get("time-pos", &sample.Time)
				_ = conn.get("duration", &sample.Duration)
				_ = conn.get("volume", &sample.Volume)
				_ = conn.get("mute", &sample.Muted)
				if sample.AudioOnly && !paused {
					var metadata map[string]string
					_ = conn.get("af-metadata/plexctl-spectrum", &metadata)
					sample.Spectrum = readSpectrum(metadata)
					sample.Waveform = Waveform{}
					_ = conn.get("user-data/plexctl-waveform", &sample.Waveform)
				}
			} else {
				sample.State = operations.StateStopped
				sample.Spectrum, sample.Waveform = Spectrum{}, Waveform{}
			}
			// IPC can deliver file boundaries between any two property replies.
			// Discard that mixed sample instead of attributing a new duration or
			// position to the old song's rating key.
			later := pm.queueEvents(&status)
			if len(later) == 0 {
				status = sample
			}
			boundaries = append(boundaries, later...)
			if previous.Key != status.Key || previous.File != status.File {
				pm.persistTrack(status)
			}
			pm.setStatus(status)
			if status.Active() && status.Overlay && (pm.redrawOverlay.Swap(false) || status.Paused() && time.Since(lastRedraw) >= 250*time.Millisecond) {
				// A paused VO otherwise never repaints after a dialog or resize.
				_ = conn.call("osd-overlay", 2, "ass-events", "", 1280, 720)
				lastRedraw = time.Now()
			}
			if err != nil {
				pm.disconnect()
			} else if idle && status.Overlay {
				_ = conn.call("quit")
				pm.disconnect()
			}
			changed := previous.State != status.State || previous.Key != status.Key
			if changed {
				pm.sendUpdate()
			}
			if len(boundaries) > 0 {
				// Preserve EOF reports even after disconnect canceled ctx. The
				// reporter drains this channel before shutting down.
				pm.enqueueReports(boundaries)
			}
			if changed || time.Since(lastReport) >= 10*time.Second || err != nil && previous.Active() {
				select {
				case pm.reports <- reportBatch{statuses: []PlayerStatus{status}}:
				default:
				}
				lastReport = time.Now()
			}
			pm.ioMu.Unlock()
			if err != nil || ctx.Err() != nil {
				return
			}
		}
	}
}

func (pm *PlayerManager) PollUpdates() tea.Cmd {
	interval := 250 * time.Millisecond
	if status := pm.Status(); status.Active() && status.AudioOnly {
		interval = 50 * time.Millisecond
	}
	return tea.Tick(interval, func(time.Time) tea.Msg { return PlayerStatusMsgPoller{} })
}

func (pm *PlayerManager) sendUpdate() {
	select {
	case pm.updates <- PlayerStatusMsg{}:
	default:
	}
}

func (pm *PlayerManager) WaitForUpdates() tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-pm.updates:
			return msg
		case <-pm.done:
			return nil
		}
	}
}

func reportProgress(ctx context.Context, status PlayerStatus) {
	if status.NoReport || status.Key == "" {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	ctx = config.MediaServer(status.Server).Context(ctx)
	client, err := plex.NewClientForContext(ctx)
	if err != nil {
		return
	}
	timePos, duration := int64(status.Time*1000), int64(status.Duration*1000)
	device := "PlexCTL"
	_, _ = client.SDK.Timeline.Report(ctx, operations.ReportRequest{
		State: &status.State, Time: &timePos, Duration: &duration,
		Key: &status.Key, RatingKey: &status.Key, Device: &device,
	})
}

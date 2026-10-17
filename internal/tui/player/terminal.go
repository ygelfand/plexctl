package player

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/LukeHagar/plexgo/models/operations"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/termgraphics"
	"golang.org/x/term"
)

// terminalPlayback owns a foreground mpv process. Bubble Tea releases its input
// and renderer for the entire Run call, then restores the library view.
// Its status is private: IPC events and progress reporting run on the same loop.
type terminalPlayback struct {
	mpv          string
	output       config.VideoOutput
	preference   config.VideoOutput
	status       PlayerStatus
	subtitles    []ExternalSubtitle
	stdin        io.Reader
	stdout       io.Writer
	report       func(context.Context, PlayerStatus)
	sharedMemory bool
	items        []PlayItem
	playlist     map[int64]PlayItem
	visualizer   string
	sixelWidth   int
	request      context.Context
	started      bool
	attempted    bool
	fallback     config.VideoOutput
}

var _ tea.ExecCommand = (*terminalPlayback)(nil)

func (pm *PlayerManager) playTerminalRequest(ctx context.Context, req *PlayRequest, preference config.VideoOutput, startOffset int64) tea.Cmd {
	server := req.Server
	item := req.Items[0]
	return func() tea.Msg {
		if ctx.Err() != nil {
			return nil
		}
		if !config.IsTUI {
			termgraphics.Probe(os.Stdin, os.Stdout)
		}
		mpv, output, err := terminalOutput(ctx, preference, os.Getenv)
		if err != nil {
			return err
		}
		playback := &terminalPlayback{
			mpv: mpv, output: output, preference: preference, subtitles: req.Subtitles, report: pm.reportTerminal, items: req.Items,
			sixelWidth: config.Get().SixelVideoWidth, request: ctx,
			status: PlayerStatus{Server: server,
				Title: item.Title, File: item.URL, Key: item.RatingKey, NoReport: req.NoReport, Duration: item.Duration,
				Time: float64(startOffset) / 1000,
			},
		}
		if req.Music {
			playback.visualizer = musicVideoGraph(config.Get().WindowMusicVisualizer)
		}
		// A separate socket prevents a later TUI from attaching to a foreground
		// player whose terminal belongs to this session.
		pm.stopCurrent(ctx)
		finished := func(err error) tea.Msg {
			pm.flushReports()
			if err != nil {
				return err
			}
			if playback.fallback != config.VideoWindow {
				return VideoFallbackMsg{From: playback.fallback}
			}
			return PlayerStatusMsg{}
		}
		if config.IsTUI {
			return tea.Exec(playback, finished)()
		}
		playback.SetStdin(os.Stdin)
		playback.SetStdout(os.Stdout)
		return finished(playback.Run())
	}
}

func (p *terminalPlayback) SetStdin(r io.Reader) { p.stdin = r }
func (p *terminalPlayback) SetStdout(w io.Writer) {
	if output, ok := w.(interface{ TerminalFile() *os.File }); ok {
		w = output.TerminalFile()
	}
	p.stdout = w
}

// mpv diagnostics can contain authenticated media URLs and corrupt graphics.
// Failures are returned to the TUI after it has reclaimed the terminal.
func (p *terminalPlayback) SetStderr(io.Writer) {}

func (p *terminalPlayback) args(socket string) []string {
	args := []string{
		// Terminal renderers scale every frame in software. mpv's default
		// high-quality scaler is needlessly expensive for terminal video.
		"--profile=sw-fast",
		"--idle=once", "--keep-open=no", "--no-resume-playback",
		"--pause=no", "--loop-file=no", "--loop-playlist=no",
		"--force-window=no", "--terminal=yes", "--input-terminal=yes",
		"--input-default-bindings=yes", "--really-quiet",
		"--vo=" + string(p.output), "--input-ipc-server=" + socket,
		"--force-media-title=" + p.status.Title,
		fmt.Sprintf("--start=%.3f", p.status.Time),
	}
	if p.output == config.VideoKitty {
		// Only use shared memory after the terminal proves it can read it.
		// This avoids transferring megabytes per frame through local tmux,
		// while retaining the portable transport over SSH.
		transport := "no"
		if p.sharedMemory {
			transport = "yes"
		}
		args = append(args, "--vo-kitty-use-shm="+transport)
		// Our control bar uses mpv's OSD compositor and IPC, independently
		// of the Lua OSC and its window-system mouse events.
		args = append(args, "--osc=no")
	}
	if p.visualizer != "" {
		args = append(args, "--lavfi-complex="+p.visualizer)
	}
	if p.fallback != config.VideoWindow {
		args = append(args, "--osd-playing-msg="+VideoFallbackMsg{From: p.fallback}.String())
	}
	return args
}

func (p *terminalPlayback) Run() (err error) {
	ctx := p.request
	if ctx == nil {
		ctx = context.Background()
	}
	// Superseded playback is a normal stop, including during output fallback.
	defer func() {
		if ctx.Err() != nil {
			err = nil
		}
	}()
	if ctx.Err() != nil {
		return nil
	}
	initial := p.status
	err = p.run()
	if ctx.Err() != nil {
		return nil
	}
	if err == nil || p.preference != config.VideoAuto || p.started || !p.attempted || p.output == config.VideoTCT {
		return err
	}
	if _, _, fallbackErr := terminalOutput(ctx, config.VideoTCT, os.Getenv); fallbackErr != nil {
		return err
	}
	p.fallback, p.output, p.preference = p.output, config.VideoTCT, config.VideoTCT
	p.status, p.playlist = initial, nil
	return p.run()
}

func (p *terminalPlayback) run() error {
	request := p.request
	if request == nil {
		request = context.Background()
	}
	ctx, cancel := context.WithCancel(request)
	defer cancel()
	startup, stopStartup := context.WithTimeout(ctx, 2*time.Minute)
	defer stopStartup()
	startupDone := startup.Done()
	for _, stream := range []any{p.stdin, p.stdout} {
		if f, ok := stream.(interface{ Fd() uintptr }); ok && !term.IsTerminal(int(f.Fd())) {
			return fmt.Errorf("terminal video requires an interactive terminal")
		}
	}
	if input, ok := p.stdin.(*os.File); ok {
		caps := termgraphics.Probe(input, p.stdout)
		p.sharedMemory = caps.KittySharedMemory
		if p.preference != config.VideoWindow {
			var err error
			p.mpv, p.output, err = terminalOutput(startup, p.preference, os.Getenv)
			if err != nil {
				return err
			}
		}
		if caps.Responded && ((p.output == config.VideoKitty && !caps.Kitty) || (p.output == config.VideoSixel && !caps.Sixel)) {
			name := caps.Name
			if name == "" {
				name = "the attached terminal"
			}
			return fmt.Errorf("%s does not advertise %s graphics; open plexctl in a supporting terminal or select Text", name, p.output)
		}
	}
	dir, err := os.MkdirTemp("", "plexctl-mpv-")
	if err != nil {
		return fmt.Errorf("create terminal player socket directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			slog.Warn("Remove terminal player socket directory", "error", err)
		}
	}()
	socket := filepath.Join(dir, "ipc")
	if runtime.GOOS == "windows" {
		socket = `\\.\pipe\` + filepath.Base(dir)
	}

	cmd := exec.CommandContext(ctx, p.mpv, p.args(socket)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = p.stdin, p.stdout, io.Discard
	var keys <-chan string
	var graphics *termgraphics.Writer
	var controls *terminalControls
	var cellW, cellH float64
	paneZoom, err := termgraphics.NewPaneZoom()
	if err != nil {
		return err
	}
	if paneZoom != nil {
		defer func() {
			if err := paneZoom.Restore(); err != nil {
				slog.Warn("Restore terminal playback pane", "error", err)
			}
		}()
	}
	if termgraphics.InTmux() && (p.output == config.VideoKitty || p.output == config.VideoSixel) {
		restore, err := termgraphics.EnablePassthrough()
		if err != nil {
			return err
		}
		defer restore()
		if p.output == config.VideoSixel {
			graphics = &termgraphics.Writer{Output: p.stdout}
			cmd.Stdout = graphics
			defer func() {
				_, _ = io.WriteString(p.stdout, "\x1b[2J\x1b[H")
			}()
		}
	}
	if p.output == config.VideoKitty {
		controls = &terminalControls{focus: -1, hover: -1, speed: 1, subtitles: true}
		controls.canFullscreen = paneZoom != nil
		if paneZoom != nil {
			controls.fullscreen, err = paneZoom.Zoomed()
			if err != nil {
				return err
			}
		}
		p.status.Volume = 100
		graphics = &termgraphics.Writer{Output: p.stdout, Direct: !termgraphics.InTmux(), ImageID: (1 << 30) + 2*rand.IntN(1<<19)}
		cmd.Stdout = graphics
		defer func() { _, _ = io.WriteString(p.stdout, termgraphics.Wrap(graphics.DeleteImages())) }()
	}
	if p.output == config.VideoSixel {
		cmd.Args = append(cmd.Args, p.sixelArgs()...)
	}
	updateOrigin := func() (bool, error) {
		if paneZoom == nil || graphics == nil {
			return false, nil
		}
		x, y, err := paneZoom.Origin()
		if err != nil {
			return false, err
		}
		return graphics.SetPaneOrigin(x, y), nil
	}
	if _, err := updateOrigin(); err != nil {
		return err
	}
	// Our wrapper handles actual tmux panes. Also remove stale tmux variables
	// from direct Kitty windows so mpv cannot wrap their graphics by mistake.
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "TMUX=") && !strings.HasPrefix(entry, "TMUX_PANE=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	if input, ok := p.stdin.(*os.File); ok && term.IsTerminal(int(input.Fd())) {
		// Own input while mpv plays: this works with piped graphics in tmux
		// and provides the same mouse controls in every terminal renderer.
		cmd.Args = append(cmd.Args, "--input-terminal=no")
		var stop func()
		keys, stop, err = readPlaybackKeys(p.stdin)
		if err != nil {
			return err
		}
		defer stop()
		_, _ = io.WriteString(p.stdout, "\x1b[?1000h\x1b[?1006h")
		defer func() { _, _ = io.WriteString(p.stdout, "\x1b[?1000l\x1b[?1006l") }()
		if controls != nil {
			cellW, cellH = terminalCellSize(input)
			// Motion supports seek dragging and button highlights. Ask the
			// actual terminal for cell metrics even over SSH.
			_, _ = io.WriteString(p.stdout, "\x1b[?1003h"+termgraphics.Wrap("\x1b[16t"))
			defer func() { _, _ = io.WriteString(p.stdout, "\x1b[?1003l") }()
		}
	}
	p.attempted = true
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start terminal player: %w", err)
	}
	done := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(done)
	}()
	waited := false
	defer func() {
		if !waited {
			_ = cmd.Process.Kill()
			<-done
		}
	}()

	conn, err := connectTerminal(startup, socket, done)
	if err != nil {
		return err
	}
	events := make(chan terminalEvent, 64)
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		readTerminalEvents(ctx, conn, events)
	}()
	defer func() {
		cancel()
		if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			slog.Debug("Close terminal player connection", "error", err)
		}
		<-readerDone
	}()

	properties := []string{"time-pos", "pause", "duration"}
	if controls != nil {
		properties = append(properties, "volume", "mute", "speed", "sub-visibility")
	}
	graphPropertyID := len(properties) + 1
	properties = append(properties, "lavfi-complex")
	videoGraph := p.visualizer
	for id, property := range properties {
		if err := sendTerminalCommand(conn, 0, "observe_property", id+1, property); err != nil {
			return err
		}
	}
	// Build the queue paused, using per-file options so resume applies only
	// to the first item. Playlist IDs identify repeated paths unambiguously.
	items := p.items
	if len(items) == 0 {
		items = []PlayItem{{URL: p.status.File, Title: p.status.Title, RatingKey: p.status.Key}}
	}
	pending := []terminalEvent{}
	for i, item := range items {
		offset := 0.0
		flags, pause := "append", "no"
		if i == 0 {
			flags, pause, offset = "replace", "yes", p.status.Time
		}
		command := map[string]any{"name": "loadfile", "url": item.URL, "flags": flags,
			"options": map[string]string{"start": fmt.Sprintf("%.3f", offset),
				"pause": pause, "force-media-title": item.Title}}
		if err := sendTerminalRequest(conn, 1, command); err != nil {
			return err
		}
		// Drain acknowledgements while building large artist queues. Otherwise
		// a full event channel can block mpv's IPC output and our next write.
		if err := awaitQueueLoad(startup, events, &pending); err != nil {
			return err
		}
	}
	if err := sendTerminalCommand(conn, 3, "get_property", "playlist"); err != nil {
		return err
	}

	loaded := false
	defer func() {
		if loaded {
			p.status.State = operations.StateStopped
			p.reportStatus()
		}
	}()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	controlTicker := time.NewTicker(100 * time.Millisecond)
	defer controlTicker.Stop()
	var controlColumns, controlRows int
	lastPaneCheck := time.Now()
	keyTimer := time.NewTimer(time.Hour)
	keyTimer.Stop()
	defer keyTimer.Stop()
	pendingKeys := ""
	forwardKeys := func(flush bool) error {
		var decoded []playbackInput
		decoded, pendingKeys = playbackInputs(pendingKeys, flush)
		for _, input := range decoded {
			if input.CellWidth > 0 {
				cellW, cellH = input.CellWidth, input.CellHeight
				continue
			}
			var command []any
			if controls != nil {
				// ioctl follows window and font changes when pixel metrics are
				// available; otherwise retain the terminal's cell-size reply.
				if f, ok := p.stdin.(*os.File); ok {
					if w, h := terminalCellSize(f); w > 0 {
						cellW, cellH = w, h
					}
				}
				frame := graphics.Frame()
				command = controls.input(input, controls.layout(frame, p.status), frame, cellW, cellH, p.status)
			} else if input.Key == "f" && paneZoom != nil {
				command = []any{"cycle", "fullscreen"}
			} else {
				key := input.Key
				if input.Mouse != nil {
					key = input.Mouse.key()
				}
				if key != "" {
					command = []any{"keypress", key}
				}
			}
			if len(command) == 0 {
				continue
			}
			if paneZoom != nil && len(command) == 2 && command[0] == "cycle" && command[1] == "fullscreen" {
				zoomed, err := paneZoom.Toggle()
				if err != nil {
					return err
				}
				if _, err := updateOrigin(); err != nil {
					return err
				}
				if controls != nil {
					controls.fullscreen = zoomed
					controls.dragging, controls.hover = false, -1
				}
				continue
			}
			if err := sendTerminalCommand(conn, 0, command...); err != nil {
				return err
			}
		}
		if pendingKeys != "" {
			keyTimer.Reset(50 * time.Millisecond)
		}
		return nil
	}
	handleEvent := func(ev terminalEvent) error {
		if ev.RequestID == 1 && ev.Error != "success" {
			return fmt.Errorf("mpv rejected the media stream")
		}
		if ev.RequestID == 3 {
			var playlist []struct {
				ID int64 `json:"id"`
			}
			if ev.Error != "success" || json.Unmarshal(ev.Data, &playlist) != nil || len(playlist) != len(items) {
				return fmt.Errorf("mpv did not accept the complete playback queue")
			}
			p.playlist = make(map[int64]PlayItem, len(items))
			for i, item := range items {
				p.playlist[playlist[i].ID] = item
			}
			if err := sendTerminalCommand(conn, 0, "set_property", "pause", false); err != nil {
				return err
			}
		}
		if ev.RequestID == 2 && ev.Error != "success" {
			return fmt.Errorf("mpv could not draw video controls; use an mpv build with libass OSD support")
		}
		switch ev.Event {
		case "start-file":
			if item, ok := p.playlist[ev.PlaylistEntryID]; ok {
				setTrack(&p.status, item)
			}
		case "file-loaded":
			// The startup deadline must not limit the running video's lifetime.
			startupDone = nil
			stopStartup()
			p.started = true
			loaded = true
			p.status.Play()
			for _, sub := range p.subtitles {
				if err := sendTerminalCommand(conn, 0, "sub-add", sub.URL, "auto", sub.Title, sub.Language); err != nil {
					return err
				}
			}
			p.reportStatus()
		case "property-change":
			if ev.ID == graphPropertyID {
				_ = json.Unmarshal(ev.Data, &videoGraph)
				return nil
			}
			oldState := p.status.State
			p.applyProperty(ev)
			if controls != nil && len(ev.Data) > 0 && string(ev.Data) != "null" {
				switch ev.ID {
				case 6:
					_ = json.Unmarshal(ev.Data, &controls.speed)
				case 7:
					_ = json.Unmarshal(ev.Data, &controls.subtitles)
				}
			}
			if loaded && p.status.State != oldState {
				p.reportStatus()
			}
		case "end-file":
			if ev.Reason == "error" {
				return fmt.Errorf("mpv could not play the stream using %s; check the server connection and terminal graphics support", p.output)
			}
			if ev.Reason == "eof" && p.status.Duration > 0 {
				p.status.Time = p.status.Duration
			}
			p.status.State = operations.StateStopped
			p.reportStatus()
			loaded = false
		case "shutdown":
			// Continue draining properties until the process exits.
		}
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-startupDone:
			return fmt.Errorf("terminal player did not finish starting playback: %w", startup.Err())
		case data, ok := <-keys:
			if !ok {
				keys = nil
				continue
			}
			pendingKeys += data
			keyTimer.Stop()
			if err := forwardKeys(false); err != nil {
				return err
			}
		case <-keyTimer.C:
			if err := forwardKeys(true); err != nil {
				return err
			}
		case <-controlTicker.C:
			if !loaded {
				continue
			}
			resized := false
			if input, ok := p.stdin.(*os.File); ok {
				if columns, rows, err := term.GetSize(int(input.Fd())); err == nil {
					resized = columns != controlColumns || rows != controlRows
					controlColumns, controlRows = columns, rows
				}
			}
			if resized || time.Since(lastPaneCheck) >= time.Second {
				moved, err := updateOrigin()
				if err != nil {
					return err
				}
				resized = resized || moved
				lastPaneCheck = time.Now()
			}
			if resized && p.output == config.VideoSixel {
				if err := resizeVideoOutput(p.output, p.sixelArgs(), videoGraph, func(args ...any) error {
					return sendTerminalCommand(conn, 0, args...)
				}); err != nil {
					return err
				}
			}
			if controls == nil {
				continue
			}
			if resized && paneZoom != nil {
				if zoomed, err := paneZoom.Zoomed(); err == nil {
					controls.fullscreen = zoomed
				}
			}
			layout := controls.layout(graphics.Frame(), p.status)
			overlay := controls.overlay(layout, p.status)
			// A paused Kitty VO needs an OSD redraw to process SIGWINCH.
			// Resending the overlay preserves pause and the playback position.
			if overlay == controls.lastOverlay && !resized {
				continue
			}
			if err := sendTerminalCommand(conn, 2, "osd-overlay", 1, "ass-events", overlay, layout.w, layout.h); err != nil {
				return err
			}
			controls.lastOverlay = overlay
		case ev, ok := <-events:
			if !ok {
				// IPC normally closes just before Wait returns on EOF or q.
				select {
				case <-done:
					waited = true
					return p.exitError(waitErr)
				case <-time.After(2 * time.Second):
					return fmt.Errorf("terminal player disconnected from IPC")
				}
			}
			// Decoder events can arrive before playlist discovery, including EOF
			// for tiny files while paused. Replay them only after IDs are known.
			if p.playlist == nil && ev.Event != "" && (ev.Event != "end-file" || ev.Reason != "error") {
				pending = append(pending, ev)
				continue
			}
			if err := handleEvent(ev); err != nil {
				return err
			}
			if p.playlist != nil && len(pending) > 0 {
				for _, buffered := range pending {
					if err := handleEvent(buffered); err != nil {
						return err
					}
				}
				pending = nil
			}
		case <-ticker.C:
			if loaded {
				p.reportStatus()
			}
		}
	}
}

func (p *terminalPlayback) sixelArgs() []string {
	columns, rows := 80, 24
	cw, ch := termgraphics.Current().CellWidth, termgraphics.Current().CellHeight
	if input, ok := p.stdin.(*os.File); ok {
		if w, h, err := term.GetSize(int(input.Fd())); err == nil {
			columns, rows = w, h
		}
		if w, h := terminalCellSize(input); w > 0 && h > 0 {
			cw, ch = int(w), int(h)
		}
	}
	return sixelSizeArgs(p.sixelWidth, columns, rows, cw, ch)
}

func awaitQueueLoad(ctx context.Context, events <-chan terminalEvent, pending *[]terminalEvent) error {
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-events:
			if !ok {
				return fmt.Errorf("mpv disconnected while building the playback queue")
			}
			if event.RequestID == 1 {
				if event.Error != "success" {
					return fmt.Errorf("mpv rejected the media stream")
				}
				return nil
			}
			if event.Event == "end-file" && event.Reason == "error" {
				return fmt.Errorf("mpv could not open a queued media stream")
			}
			if event.Event != "" {
				*pending = append(*pending, event)
			}
		case <-timer.C:
			return fmt.Errorf("mpv did not acknowledge the queued media stream")
		}
	}
}

func (p *terminalPlayback) exitError(err error) error {
	var exit *exec.ExitError
	// mpv uses status 4 for a user interrupt (Ctrl+C).
	if errors.As(err, &exit) && exit.ExitCode() == 4 {
		return nil
	}
	if err != nil {
		return fmt.Errorf("terminal playback with %s failed; check terminal support and 'mpv --vo=help': %w", p.output, err)
	}
	return nil
}

func (p *terminalPlayback) reportStatus() {
	if p.report == nil || p.status.NoReport || p.status.Key == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	p.report(ctx, p.status)
}

type terminalEvent struct {
	PlaylistEntryID int64           `json:"playlist_entry_id"`
	Event           string          `json:"event"`
	ID              int             `json:"id"`
	Data            json.RawMessage `json:"data"`
	Reason          string          `json:"reason"`
	RequestID       int             `json:"request_id"`
	Error           string          `json:"error"`
}

func (p *terminalPlayback) applyProperty(ev terminalEvent) {
	if len(ev.Data) == 0 || string(ev.Data) == "null" {
		return
	}
	switch ev.ID {
	case 1:
		_ = json.Unmarshal(ev.Data, &p.status.Time)
	case 2:
		var paused bool
		if json.Unmarshal(ev.Data, &paused) == nil {
			if paused {
				p.status.State = operations.StatePaused
			} else {
				p.status.Play()
			}
		}
	case 3:
		_ = json.Unmarshal(ev.Data, &p.status.Duration)
	case 4:
		_ = json.Unmarshal(ev.Data, &p.status.Volume)
	case 5:
		_ = json.Unmarshal(ev.Data, &p.status.Muted)
	}
}

func connectTerminal(ctx context.Context, socket string, done <-chan struct{}) (net.Conn, error) {
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	retry := time.NewTicker(50 * time.Millisecond)
	defer retry.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-done:
			return nil, fmt.Errorf("mpv exited before terminal playback started; check 'mpv --vo=help'")
		case <-timeout.C:
			return nil, fmt.Errorf("timed out connecting to the terminal player")
		case <-retry.C:
			if conn, err := dialTerminal(socket); err == nil {
				return conn, nil
			}
		}
	}
}

func sendTerminalCommand(conn net.Conn, requestID int, args ...any) error {
	return sendTerminalRequest(conn, requestID, args)
}

func sendTerminalRequest(conn net.Conn, requestID int, command any) error {
	if err := conn.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return fmt.Errorf("set player IPC deadline: %w", err)
	}
	request := map[string]any{"command": command, "request_id": requestID}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return fmt.Errorf("send player IPC command: %w", err)
	}
	return nil
}

func readTerminalEvents(ctx context.Context, conn net.Conn, events chan<- terminalEvent) {
	defer close(events)
	decoder := json.NewDecoder(conn)
	for {
		var ev terminalEvent
		if err := decoder.Decode(&ev); err != nil {
			return
		}
		select {
		case events <- ev:
		case <-ctx.Done():
			return
		}
	}
}

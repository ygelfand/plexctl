package player

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/LukeHagar/plexgo/models/operations"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/dexterlb/mpvipc"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
)

type PlayerManager struct {
	conn       *mpvipc.Connection
	socketPath string
	status     PlayerStatus
	stopChan   chan struct{}
	updates    chan tea.Msg
}

var (
	instance *PlayerManager
	once     sync.Once
)

func GetPlayerManager() *PlayerManager {
	once.Do(func() {
		cfg := config.Get()
		socketPath := filepath.Join(cfg.CacheDir, "mpv.sock")
		if runtime.GOOS == "windows" {
			socketPath = `\\.\pipe\mpv-socket`
		} else {
			_ = os.MkdirAll(filepath.Dir(socketPath), 0o755)
		}

		instance = &PlayerManager{
			socketPath: socketPath,
			updates:    make(chan tea.Msg, 10),
		}
	})
	return instance
}

// Lifecycle Methods

func (pm *PlayerManager) VerifyConnection() bool {
	if pm.conn == nil {
		return false
	}
	// Try to get a basic property with a short timeout to ensure mpv is "real"
	done := make(chan bool, 1)
	go func() {
		_, err := pm.conn.Call("get_property", "mpv-version")
		done <- err == nil
	}()

	select {
	case ok := <-done:
		return ok
	case <-time.After(200 * time.Millisecond):
		slog.Debug("PlayerManager: connection check timed out")
		return false
	}
}

func (pm *PlayerManager) Status() PlayerStatus {
	return pm.status
}

func (pm *PlayerManager) Play(url, title, ratingKey string, noReport bool, tctMode bool, startOffset int64) tea.Cmd {
	return func() tea.Msg {
		slog.Debug("PlayerManager.Play start", "title", title, "rk", ratingKey, "tct", tctMode, "offset", startOffset)
		if err := pm.ensureMpv(); err != nil {
			slog.Error("PlayerManager: mpv check failed", "error", err)
			return err
		}

		// Strictly validate existing connection
		if pm.conn != nil && !pm.VerifyConnection() {
			slog.Debug("PlayerManager: cleaning up non-responsive connection")
			pm.cleanup()
		}

		if pm.conn != nil {
			if pm.status.TctMode != tctMode {
				slog.Debug("PlayerManager: Mode changed, restarting mpv", "old", pm.status.TctMode, "new", tctMode)
				pm.conn.Call("quit")
				pm.cleanup()
			} else if pm.status.Key != "" && pm.status.Key != ratingKey {
				slog.Debug("PlayerManager: different media, reporting progress first")
				pm.reportProgressWithKey()
			}
		}

		if pm.conn == nil {
			slog.Debug("PlayerManager: spawning new mpv instance or re-opening connection")
			if pm.socketExists() {
				slog.Debug("PlayerManager: socket exists, attempting to connect")
				c := mpvipc.NewConnection(pm.socketPath)
				if err := c.Open(); err == nil {
					pm.conn = c
					if !pm.VerifyConnection() {
						slog.Debug("PlayerManager: connected but IPC non-responsive, cleaning up")
						pm.cleanup()
					} else {
						pm.stopChan = make(chan struct{})
						go pm.monitorEvents()
						pm.restoreStatusFromMpv()
					}
				} else {
					slog.Debug("PlayerManager: could not open socket, removing", "error", err)
					os.Remove(pm.socketPath)
				}
			}

			if pm.conn == nil {
				if err := pm.spawnMpv(tctMode); err != nil {
					slog.Error("PlayerManager: spawn failed", "error", err)
					return err
				}
			}
		}

		pm.status = PlayerStatus{
			Title:    title,
			File:     url,
			Key:      ratingKey,
			NoReport: noReport,
			TctMode:  tctMode,
			Time:     float64(startOffset) / 1000.0,
		}
		pm.status.Play()

		startSec := float64(startOffset) / 1000.0
		slog.Debug("PlayerManager: sending loadfile to mpv", "url", url, "startSec", startSec)

		pm.conn.Call("loadfile", url, "replace", "-1", fmt.Sprintf("start=%.3f", startSec))
		pm.conn.Call("set_property", "pause", false)
		pm.conn.Call("set_property", "force-media-title", title)
		pm.conn.Call("set_property", "user-data/plex-rating-key", ratingKey)
		pm.conn.Call("set_property", "user-data/plex-title", title)
		pm.conn.Call("set_property", "user-data/plex-no-report", noReport)
		pm.conn.Call("show-text", "PlexCTL: Loading "+title+"...", 5000)

		slog.Debug("PlayerManager: playback initiated")
		pm.reportProgressWithKey()
		pm.sendUpdate()

		return PlayerStatusMsg{}
	}
}

func (pm *PlayerManager) Reconnect() tea.Cmd {
	return func() tea.Msg {
		if pm.conn != nil || !pm.socketExists() {
			return nil
		}

		c := mpvipc.NewConnection(pm.socketPath)
		if err := c.Open(); err != nil {
			os.Remove(pm.socketPath)
			return nil
		}

		pm.conn = c
		if !pm.VerifyConnection() {
			slog.Debug("PlayerManager: re-connected but IPC non-responsive, cleaning up")
			pm.cleanup()
			return nil
		}

		pm.stopChan = make(chan struct{})
		pm.restoreStatusFromMpv()
		go pm.monitorEvents()

		return nil
	}
}

func (pm *PlayerManager) TogglePause() tea.Cmd {
	return func() tea.Msg {
		if !pm.VerifyConnection() {
			return nil
		}

		if paused, err := pm.conn.Call("get_property", "pause"); err == nil && paused != nil {
			newState := !paused.(bool)
			pm.conn.Call("set_property", "pause", newState)
			if newState {
				pm.status.State = operations.StatePaused
			} else {
				pm.status.State = operations.StatePlaying
			}
			pm.sendUpdate()
		}
		return nil
	}
}

func (pm *PlayerManager) Stop() tea.Cmd {
	return func() tea.Msg {
		if pm.VerifyConnection() {
			pm.conn.Call("quit")
			pm.cleanup()
		}
		return tea.Quit()
	}
}

func (pm *PlayerManager) StopPlayback() tea.Cmd {
	return func() tea.Msg {
		if pm.VerifyConnection() {
			pm.conn.Call("quit")
			pm.cleanup()
		} else {
			// Even if connection is lost, try to clean up local state
			pm.cleanup()
		}
		return PlayerStatusMsg{} // Trigger UI update to hide controls
	}
}

func (pm *PlayerManager) PollUpdates() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		if !pm.status.Active() {
			return nil
		}
		return PlayerStatusMsgPoller{}
	})
}

func (pm *PlayerManager) sendUpdate() {
	pm.updates <- PlayerStatusMsg{}
}

func (pm *PlayerManager) WaitForUpdates() tea.Cmd {
	return func() tea.Msg {
		msg := <-pm.updates
		return msg
	}
}

// Internal Helpers

func (pm *PlayerManager) ensureMpv() error {
	_, err := exec.LookPath("mpv")
	if err != nil {
		return fmt.Errorf("mpv command not found. please install mpv: %w", err)
	}
	return nil
}

func (pm *PlayerManager) socketExists() bool {
	if runtime.GOOS == "windows" {
		_, err := os.OpenFile(pm.socketPath, os.O_RDWR, 0)
		if err != nil {
			if pe, ok := err.(*os.PathError); ok {
				if errno, ok := pe.Err.(syscall.Errno); ok {
					if errno == 2 {
						return false
					}
				}
			}
			return true
		}
		return true // Named pipes are harder to stat, let dialing handle it
	}
	_, err := os.Stat(pm.socketPath)
	return err == nil
}

func (pm *PlayerManager) spawnMpv(tctMode bool) error {
	args := []string{"--idle", "--no-resume-playback", fmt.Sprintf("--input-ipc-server=%s", pm.socketPath)}
	if tctMode {
		args = append(args, "-vo", "tct", "--really-quiet", "--vo-tct-buffering=frame")
		if config.IsTUI {
			args = append(args, "--vo-tct-height=40")
		}
	} else {
		args = append(args, "--force-window=yes")
	}

	slog.Debug("PlayerManager: spawning mpv", "args", args)
	cmd := exec.Command("mpv", args...)
	if tctMode {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start mpv: %w", err)
	}

	// Wait for socket to appear and respond to IPC
	for i := 0; i < 50; i++ {
		if pm.socketExists() {
			c := mpvipc.NewConnection(pm.socketPath)
			if err := c.Open(); err == nil {
				pm.conn = c
				// Crucial: wait for IPC to actually respond
				if pm.VerifyConnection() {
					pm.stopChan = make(chan struct{})
					go pm.monitorEvents()
					return nil
				}
				pm.conn.Close()
				pm.conn = nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("failed to connect to mpv IPC after spawning")
}

func (pm *PlayerManager) setupObservers() {
	if pm.conn != nil {
		pm.conn.Call("observe_property", 1, "time-pos")
		pm.conn.Call("observe_property", 2, "pause")
		pm.conn.Call("observe_property", 3, "duration")
	}
}

func (pm *PlayerManager) monitorEvents() {
	pm.setupObservers()
	events := make(chan *mpvipc.Event)
	stop := make(chan struct{})

	go func() {
		if pm.conn != nil {
			pm.conn.ListenForEvents(events, stop)
		}
	}()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	defer func() {
		rk := pm.status.Key
		nr := pm.status.NoReport
		pm.cleanup()
		pm.sendUpdate()
		if rk != "" && !nr {
			pm.reportProgressWithKey()
		}
	}()

	for {
		select {
		case <-pm.stopChan:
			close(stop)
			return
		case ev, ok := <-events:
			if !ok || ev.Name == "shutdown" {
				slog.Debug("PlayerManager: mpv shutdown detected")
				return
			}
			if ev.Name == "end-file" {
				slog.Debug("PlayerManager: playback finished (end-file)")
				pm.status.State = operations.StateStopped
				pm.reportProgressWithKey()
			}
			if ev.Name == "property-change" {
				slog.Log(context.Background(), config.LevelTrace, "PlayerManager: property change", "id", ev.ID, "data", ev.Data)
				pm.handlePropertyChange(ev)
			}
		case <-ticker.C:
			if pm.status.Active() && pm.status.Key != "" {
				slog.Log(context.Background(), config.LevelTrace, "PlayerManager: periodic progress report")
				pm.reportProgressWithKey()
			}
		}
	}
}

func (pm *PlayerManager) handlePropertyChange(ev *mpvipc.Event) {
	if ev.Data == nil {
		return
	}
	switch ev.ID {
	case 1: // time-pos
		if val, ok := ev.Data.(float64); ok {
			pm.status.Time = val
		}
	case 2: // pause
		if val, ok := ev.Data.(bool); ok {
			if val {
				pm.status.State = operations.StatePaused
			} else {
				pm.status.State = operations.StatePlaying
			}
		}
		pm.sendUpdate()
	case 3: // duration
		if val, ok := ev.Data.(float64); ok {
			pm.status.Duration = val
		}
		pm.sendUpdate()
	}
}

func (pm *PlayerManager) restoreStatusFromMpv() {
	if pm.conn == nil {
		return
	}
	title, _ := pm.conn.Call("get_property", "user-data/plex-title")
	if title == nil {
		title, _ = pm.conn.Call("get_property", "media-title")
	}
	key, _ := pm.conn.Call("get_property", "user-data/plex-rating-key")
	path, _ := pm.conn.Call("get_property", "path")
	paused, _ := pm.conn.Call("get_property", "pause")
	idle, _ := pm.conn.Call("get_property", "idle-active")
	timePos, _ := pm.conn.Call("get_property", "time-pos")
	duration, _ := pm.conn.Call("get_property", "duration")
	noReport, _ := pm.conn.Call("get_property", "user-data/plex-no-report")

	if title != nil {
		pm.status.Title, _ = title.(string)
	}
	if key != nil {
		pm.status.Key, _ = key.(string)
	}
	if noReport != nil {
		if nrB, ok := noReport.(bool); ok {
			pm.status.NoReport = nrB
		}
	}
	if path != nil {
		pm.status.File, _ = path.(string)
	}
	if idle != nil {
		if idleB, ok := idle.(bool); ok && idleB {
			pm.status.State = operations.StateStopped
		} else {
			if paused != nil {
				if pausedB, ok := paused.(bool); ok && pausedB {
					pm.status.State = operations.StatePaused
				} else {
					pm.status.State = operations.StatePlaying
				}
			}
		}
	}
	if timePos != nil {
		pm.status.Time, _ = timePos.(float64)
	}
	if duration != nil {
		pm.status.Duration, _ = duration.(float64)
	}
}

func (pm *PlayerManager) cleanup() {
	if pm.stopChan != nil {
		select {
		case <-pm.stopChan:
		default:
			close(pm.stopChan)
		}
		pm.stopChan = nil
	}
	if pm.conn != nil {
		pm.conn.Close()
		pm.conn = nil
	}
	pm.status = PlayerStatus{}
	if runtime.GOOS != "windows" {
		os.Remove(pm.socketPath)
	}
}

func (pm *PlayerManager) reportProgressWithKey() {
	pm.sendUpdate()
	if pm.status.NoReport || pm.status.Key == "" {
		return
	}

	timePos := int64(pm.status.Time * 1000)
	duration := int64(pm.status.Duration * 1000)

	client, err := plex.NewClient()
	if err != nil {
		return
	}

	st := operations.State(pm.status.State)
	forNow := "PlexCTL"
	req := operations.ReportRequest{
		State:     &st,
		Time:      &timePos,
		Duration:  &duration,
		Key:       &pm.status.Key,
		RatingKey: &pm.status.Key,
		Device:    &forNow,
	}
	_, _ = client.SDK.Timeline.Report(context.Background(), req)
}

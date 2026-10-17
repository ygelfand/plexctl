//go:build integration && !windows

package player

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/LukeHagar/plexgo/models/operations"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ygelfand/plexctl/internal/config"
)

// Use this test binary as a small mpv IPC peer, without a display, Plex server,
// installed mpv, or network access. Run with go test -tags=integration.
func fakeTerminalMPV(t *testing.T, scenario string) string {
	t.Helper()
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "mpv")
	script := "#!/bin/sh\nexec \"$PLEXCTL_TEST_BINARY\" -test.run=^TestTerminalMPVHelper$ -- \"$@\"\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PLEXCTL_TEST_BINARY", binary)
	t.Setenv("PLEXCTL_MPV_HELPER", scenario)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return path
}

func TestTerminalPlayback(t *testing.T) {
	tests := []struct {
		name     string
		noReport bool
		wantTime float64
		wantErr  bool
		auto     bool
	}{
		{name: "eof", wantTime: 120},
		{name: "quit", wantTime: 31.5},
		{name: "trailer", noReport: true},
		{name: "startup-failure", wantErr: true},
		{name: "auto-startup-failure", auto: true, wantTime: 120},
		{name: "load-failure", wantErr: true},
		{name: "stream-failure", wantErr: true},
		{name: "controls", wantTime: 120},
		{name: "controls-failure", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reports []PlayerStatus
			p := &terminalPlayback{
				mpv: fakeTerminalMPV(t, tt.name), output: config.VideoKitty,
				stdin: strings.NewReader(""), stdout: io.Discard,
				status:    PlayerStatus{Title: "A film -- with spaces", File: "https://plex.invalid/video?X-Plex-Token=secret", Key: "42", Time: 12.345, NoReport: tt.noReport},
				subtitles: []ExternalSubtitle{{URL: "https://plex.invalid/sub?token=secret", Title: "English CC", Language: "eng"}},
				report: func(ctx context.Context, status PlayerStatus) {
					if _, ok := ctx.Deadline(); !ok {
						t.Error("progress report has no deadline")
					}
					reports = append(reports, status)
				},
			}
			if tt.auto {
				p.preference = config.VideoAuto
			}
			err := p.Run()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Run error = %v, want error %v", err, tt.wantErr)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("playback error exposed an authentication token")
			}
			if tt.auto && (p.fallback != config.VideoKitty || p.output != config.VideoTCT) {
				t.Fatalf("automatic graphics startup did not fall back to text: %+v", p)
			}
			if tt.wantErr {
				return
			}
			if tt.noReport {
				if len(reports) != 0 {
					t.Fatalf("trailer sent %d progress reports", len(reports))
				}
				return
			}
			if len(reports) < 3 {
				t.Fatalf("missing playback reports: %+v", reports)
			}
			last := reports[len(reports)-1]
			if last.State != operations.StateStopped || last.Time != tt.wantTime || last.Duration != 120 || last.Key != "42" {
				t.Errorf("incorrect final progress: %+v", last)
			}
			if !slices.ContainsFunc(reports, func(s PlayerStatus) bool { return s.Paused() }) {
				t.Error("pause was not reported")
			}
		})
	}
}

type terminalTestModel struct {
	command tea.Cmd
	err     error
	done    bool
}

func (m *terminalTestModel) Init() tea.Cmd { return m.command }
func (m *terminalTestModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case error:
		m.err, m.done = msg, true
		return m, tea.Quit
	case PlayerStatusMsg:
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}
func (m *terminalTestModel) View() string {
	if m.done {
		return "library restored"
	}
	return "library before playback"
}

func TestTerminalPlaybackRestoresTUI(t *testing.T) {
	wasTUI := config.IsTUI
	config.IsTUI = true
	t.Cleanup(func() { config.IsTUI = wasTUI })
	for _, scenario := range []string{"tui", "startup-failure"} {
		t.Run(scenario, func(t *testing.T) {
			fakeTerminalMPV(t, scenario)
			pm := newPlayerManager("")
			t.Cleanup(pm.Close)
			m := &terminalTestModel{command: pm.Play("https://plex.invalid/video", "Film", "", true, config.VideoKitty, 0, nil)}
			var output bytes.Buffer
			program := tea.NewProgram(m, tea.WithInput(strings.NewReader("")), tea.WithOutput(&output), tea.WithoutSignalHandler())
			if _, err := program.Run(); err != nil {
				t.Fatal(err)
			}
			if !m.done || (m.err != nil) != (scenario == "startup-failure") {
				t.Fatalf("playback did not return to TUI: done=%v error=%v", m.done, m.err)
			}
			view := output.String()
			if !strings.Contains(view, "library restored") {
				t.Fatalf("library not restored: %q", view)
			}
			if scenario == "startup-failure" {
				return
			}
			start, end := strings.Index(view, "video-start"), strings.Index(view, "video-end")
			if start < 0 || end < start || !strings.Contains(view[end:], "library restored") {
				t.Fatalf("missing terminal handoff or restoration: %q", view)
			}
			if strings.Contains(view[start:end], "library") {
				t.Fatal("TUI rendered over video")
			}
		})
	}
}

func TestTerminalMPVHelper(t *testing.T) {
	scenario := os.Getenv("PLEXCTL_MPV_HELPER")
	if scenario == "" {
		return
	}
	if slices.Contains(os.Args, "--vo=help") {
		fmt.Println("Available video outputs:\n  kitty graphics\n  sixel graphics\n  tct text")
		os.Exit(0)
	}
	if scenario == "startup-failure" || scenario == "auto-startup-failure" && slices.Contains(os.Args, "--vo=kitty") {
		os.Exit(3)
	}
	if !slices.Contains(os.Args, "--idle=once") || (scenario != "auto-startup-failure" && (!slices.Contains(os.Args, "--vo=kitty") || !slices.Contains(os.Args, "--vo-kitty-use-shm=no"))) {
		os.Exit(10)
	}
	if scenario != "tui" && !slices.Contains(os.Args, "--start=12.345") {
		os.Exit(11)
	}
	var socket string
	for _, arg := range os.Args {
		if value, ok := strings.CutPrefix(arg, "--input-ipc-server="); ok {
			socket = value
		}
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		os.Exit(12)
	}
	t.Cleanup(func() {
		if err := listener.Close(); err != nil {
			t.Error(err)
		}
	})
	conn, err := listener.Accept()
	if err != nil {
		os.Exit(13)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	})
	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)
	loaded, playlist, unpaused, subtitle := false, false, false, scenario == "tui"
	for !loaded || !playlist || !unpaused || !subtitle {
		var request struct {
			Command   json.RawMessage `json:"command"`
			RequestID int             `json:"request_id"`
		}
		if decoder.Decode(&request) != nil {
			os.Exit(14)
		}
		var named map[string]any
		if json.Unmarshal(request.Command, &named) == nil && named["name"] == "loadfile" {
			if scenario == "load-failure" {
				_ = encoder.Encode(map[string]any{"request_id": request.RequestID, "error": "invalid parameter"})
				_, _ = io.Copy(io.Discard, conn)
				return
			}
			_ = encoder.Encode(map[string]any{"request_id": request.RequestID, "error": "success"})
			if scenario == "stream-failure" {
				_ = encoder.Encode(map[string]any{"event": "end-file", "reason": "error"})
				return
			}
			_ = encoder.Encode(map[string]any{"event": "file-loaded"})
			loaded = true
			continue
		}
		var command []any
		if json.Unmarshal(request.Command, &command) != nil || len(command) == 0 {
			os.Exit(14)
		}
		switch command[0] {
		case "get_property":
			if len(command) == 2 && command[1] == "playlist" {
				_ = encoder.Encode(map[string]any{"request_id": request.RequestID, "error": "success", "data": []any{map[string]any{"id": 1}}})
				playlist = true
			}
		case "set_property":
			if len(command) == 3 && command[1] == "pause" && command[2] == false {
				unpaused = true
			}
		case "sub-add":
			if len(command) != 5 || command[3] != "English CC" || command[4] != "eng" {
				os.Exit(15)
			}
			subtitle = true
		}
	}

	for _, property := range []struct {
		id   int
		data any
	}{{3, 120}, {1, 31.5}, {2, true}, {2, false}} {
		_ = encoder.Encode(map[string]any{"event": "property-change", "id": property.id, "data": property.data})
	}
	if strings.HasPrefix(scenario, "controls") {
		for id, data := range map[int]any{4: 65, 5: true, 6: 1.25, 7: false} {
			_ = encoder.Encode(map[string]any{"event": "property-change", "id": id, "data": data})
		}
		// The geometry is consumed by the real output adapter, without an
		// emulator. Wait for a live overlay on the same playback IPC connection.
		fmt.Print("\x1b[3;1H\x1b_Ga=T,f=24,s=960,v=540;AAAA\x1b\\")
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		for {
			var request struct {
				Command   []any `json:"command"`
				RequestID int   `json:"request_id"`
			}
			if decoder.Decode(&request) != nil {
				os.Exit(16)
			}
			if request.Command[0] != "osd-overlay" {
				continue
			}
			if len(request.Command) != 6 || request.Command[2] != "ass-events" {
				os.Exit(17)
			}
			overlay, _ := request.Command[3].(string)
			for _, label := range []string{"Return", "Muted", "CC off", "1.25x", "0:31 / 2:00"} {
				if !strings.Contains(overlay, label) {
					os.Exit(18)
				}
			}
			if strings.Contains(overlay, "secret") {
				os.Exit(19)
			}
			err := "success"
			if scenario == "controls-failure" {
				err = "unsupported"
			}
			_ = encoder.Encode(map[string]any{"request_id": request.RequestID, "error": err})
			if scenario == "controls-failure" {
				_, _ = io.Copy(io.Discard, conn)
				return
			}
			break
		}
	}
	if scenario == "tui" {
		fmt.Println("video-start")
		time.Sleep(100 * time.Millisecond)
		fmt.Println("video-end")
	}
	reason := "eof"
	if scenario == "quit" {
		reason = "quit"
	}
	_ = encoder.Encode(map[string]any{"event": "end-file", "reason": reason})
	_ = encoder.Encode(map[string]any{"event": "shutdown"})
	// Avoid the testing harness printing PASS into the simulated video stream.
	_ = conn.Close()
	_ = listener.Close()
	os.Exit(0)
}

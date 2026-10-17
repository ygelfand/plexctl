//go:build integration && !windows

package player

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/ygelfand/plexctl/internal/config"
)

func TestTerminalPlaybackStopsWhenRequestIsCanceled(t *testing.T) {
	mpv, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv is not installed")
	}
	for _, fallback := range []bool{false, true} {
		t.Run(fmt.Sprintf("fallback=%v", fallback), func(t *testing.T) {
			dir := t.TempDir()
			wrapper := filepath.Join(dir, "mpv")
			script := "#!/bin/sh\nfor arg; do [ \"$arg\" = --vo=kitty ] && exit 3; done\nexec '" + strings.ReplaceAll(mpv, "'", "'\\''") + "' --no-config --ao=null \"$@\"\n"
			if err := os.WriteFile(wrapper, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			playback := &terminalPlayback{
				mpv: wrapper, output: config.VideoOutput("null"), stdin: strings.NewReader(""), stdout: io.Discard,
				request: ctx, items: []PlayItem{{URL: testAudio(t, dir), RatingKey: "cancel-me"}},
			}
			if fallback {
				playback.output, playback.preference = config.VideoKitty, config.VideoAuto
			}
			setTrack(&playback.status, playback.items[0])
			var canceledAt time.Time
			playback.report = func(_ context.Context, status PlayerStatus) {
				if status.State == operations.StatePlaying && canceledAt.IsZero() {
					canceledAt = time.Now()
					cancel()
				}
			}
			if err := playback.Run(); err != nil {
				t.Fatal(err)
			}
			if canceledAt.IsZero() {
				t.Fatal("player never started")
			}
			if elapsed := time.Since(canceledAt); elapsed > time.Second {
				t.Fatalf("canceled playback kept running for %v", elapsed)
			}
		})
	}
}

func summarizeReports(statuses []PlayerStatus) []timelineReport {
	reports := make([]timelineReport, 0, len(statuses))
	for _, status := range statuses {
		reports = append(reports, timelineReport{key: status.Key, state: string(status.State),
			time: int64(status.Time * 1000), duration: int64(status.Duration * 1000)})
	}
	return reports
}

func TestTerminalQueueReportsEveryTrack(t *testing.T) {
	isolatePlayerCache(t)
	mpv, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv is not installed")
	}
	dir := t.TempDir()
	wrapper := filepath.Join(dir, "mpv")
	script := "#!/bin/sh\nexec '" + strings.ReplaceAll(mpv, "'", "'\\''") + "' --no-config --ao=null \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	media := queueTestAudio(t, dir)
	playback := &terminalPlayback{
		mpv: wrapper, output: config.VideoOutput("null"), stdin: strings.NewReader(""), stdout: io.Discard,
	}
	for i := range 8 {
		playback.items = append(playback.items, PlayItem{
			URL: media, Title: fmt.Sprintf("Track %d", i), RatingKey: fmt.Sprint(i), Duration: .02,
		})
	}
	setTrack(&playback.status, playback.items[0])
	playback.status.Time = .01 // Resume only the first entry.
	reports := []PlayerStatus{}
	playback.report = func(_ context.Context, status PlayerStatus) { reports = append(reports, status) }
	if err := playback.Run(); err != nil {
		t.Fatal(err)
	}
	completed := make(map[string]bool)
	for _, status := range reports {
		if status.State != "stopped" {
			continue
		}
		if status.Time != .02 || status.Duration != .02 {
			t.Errorf("incorrect track completion: key=%q time=%g duration=%g", status.Key, status.Time, status.Duration)
		}
		completed[status.Key] = true
	}
	for _, item := range playback.items {
		if !completed[item.RatingKey] {
			t.Errorf("missing completion for track %q; reports %+v", item.RatingKey, summarizeReports(reports))
		}
	}
}

func TestTerminalQueueRetainsEventsBeforePlaylistDiscovery(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PLEXCTL_QUEUE_TEST_BINARY", binary)
	t.Setenv("PLEXCTL_QUEUE_HELPER", "early-events")
	wrapper := filepath.Join(t.TempDir(), "mpv")
	script := "#!/bin/sh\nexec \"$PLEXCTL_QUEUE_TEST_BINARY\" -test.run=^TestTerminalQueueMPVHelper$ -- \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	playback := &terminalPlayback{
		mpv: wrapper, output: config.VideoOutput("null"), stdin: strings.NewReader(""), stdout: io.Discard,
	}
	for i := range 3 {
		playback.items = append(playback.items, PlayItem{URL: "same-file", RatingKey: fmt.Sprint(i), Duration: 1})
	}
	setTrack(&playback.status, playback.items[0])
	reports := []PlayerStatus{}
	playback.report = func(_ context.Context, status PlayerStatus) { reports = append(reports, status) }
	if err := playback.Run(); err != nil {
		t.Fatal(err)
	}
	completed := make(map[string]bool)
	for _, status := range reports {
		if status.State == "stopped" && status.Time == 1 {
			completed[status.Key] = true
		}
	}
	for _, item := range playback.items {
		if !completed[item.RatingKey] {
			t.Errorf("lost pre-discovery completion for %q; reports %+v", item.RatingKey, summarizeReports(reports))
		}
	}
}

func TestTerminalQueueMPVHelper(t *testing.T) {
	if os.Getenv("PLEXCTL_QUEUE_HELPER") != "early-events" {
		return
	}
	var socket string
	for _, arg := range os.Args {
		if value, ok := strings.CutPrefix(arg, "--input-ipc-server="); ok {
			socket = value
		}
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		os.Exit(10)
	}
	conn, err := listener.Accept()
	if err != nil {
		os.Exit(11)
	}
	decoder, encoder := json.NewDecoder(conn), json.NewEncoder(conn)
	complete := func(id int64) {
		for _, event := range []terminalEvent{
			{Event: "start-file", PlaylistEntryID: id},
			{Event: "file-loaded"},
			{Event: "end-file", PlaylistEntryID: id, Reason: "eof"},
		} {
			if encoder.Encode(event) != nil {
				os.Exit(12)
			}
		}
	}
	loaded := 0
	for {
		var request struct {
			ID      int             `json:"request_id"`
			Command json.RawMessage `json:"command"`
		}
		if decoder.Decode(&request) != nil {
			os.Exit(13)
		}
		var command []any
		var data any
		playlist := json.Unmarshal(request.Command, &command) == nil &&
			len(command) > 1 && command[0] == "get_property" && command[1] == "playlist"
		if playlist {
			data = []map[string]int64{{"id": 10}, {"id": 11}, {"id": 12}}
		}
		if encoder.Encode(map[string]any{"request_id": request.ID, "error": "success", "data": data}) != nil {
			os.Exit(14)
		}
		var named struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(request.Command, &named) == nil && named.Name == "loadfile" {
			loaded++
			if loaded <= 2 {
				complete(int64(loaded + 9))
			}
		}
		if playlist {
			complete(12)
			// Keep IPC open until the caller has issued its initial unpause.
			if decoder.Decode(&request) != nil ||
				encoder.Encode(map[string]any{"request_id": request.ID, "error": "success"}) != nil {
				os.Exit(15)
			}
			_ = conn.Close()
			_ = listener.Close()
			os.Exit(0)
		}
	}
}

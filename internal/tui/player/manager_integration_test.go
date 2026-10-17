//go:build integration && !windows

package player

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ygelfand/plexctl/internal/config"
)

func queueTestAudio(t *testing.T, dir string) string {
	t.Helper()
	const rate, frames = 48000, 960 // 20 ms, shorter than a player monitor tick.
	var wav bytes.Buffer
	wav.WriteString("RIFF")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(36+frames*4))
	wav.WriteString("WAVEfmt ")
	for _, value := range []any{uint32(16), uint16(1), uint16(2), uint32(rate), uint32(rate * 4), uint16(4), uint16(16)} {
		_ = binary.Write(&wav, binary.LittleEndian, value)
	}
	wav.WriteString("data")
	_ = binary.Write(&wav, binary.LittleEndian, uint32(frames*4))
	wav.Write(make([]byte, frames*4))
	path := filepath.Join(dir, "short.wav")
	if err := os.WriteFile(path, wav.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPlaybackQueueReportsEveryShortRepeatedPath(t *testing.T) {
	isolatePlayerCache(t)
	mpv, err := exec.LookPath("mpv")
	if err != nil {
		t.Skip("mpv is not installed")
	}
	// Unix socket paths need to remain short even under long test artifact roots.
	dir, err := os.MkdirTemp("", "plexctl-queue-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	media := queueTestAudio(t, dir)
	socket := filepath.Join(dir, "ipc")
	cmd := exec.Command(mpv, "--no-config", "--ao=null", "--vo=null", "--idle=yes",
		"--force-window=no", "--input-ipc-server="+socket)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(socket); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("mpv did not open its IPC socket")
		}
		time.Sleep(10 * time.Millisecond)
	}
	server, reports := timelineServer(t)
	request := &PlayRequest{Server: server, Music: true}
	for i := range 8 {
		request.Items = append(request.Items, PlayItem{
			URL: media, Title: fmt.Sprintf("Track %d", i), RatingKey: fmt.Sprint(i), Duration: .02,
		})
	}
	pm := newPlayerManager(socket)
	t.Cleanup(func() { pm.StopPlayback()(); pm.Close() })
	if message := pm.PlayRequest(request, config.AudioTerminal, 0)(); message != nil {
		t.Fatalf("start queue: %v", message)
	}
	completed := make(map[string]bool)
	timeout := time.NewTimer(6 * time.Second)
	defer timeout.Stop()
	for len(completed) < len(request.Items) {
		select {
		case report := <-reports:
			if report.state != "stopped" {
				continue
			}
			if report.time != 20 || report.duration != 20 {
				t.Errorf("incorrect final timeline: %+v", report)
			}
			completed[report.key] = true
		case <-timeout.C:
			status := pm.Status()
			t.Fatalf("missing track completions: got %v; state=%q key=%q time=%g", completed, status.State, status.Key, status.Time)
		}
	}
	for _, item := range request.Items {
		if !completed[item.RatingKey] {
			t.Errorf("missing completion for track %q", item.RatingKey)
		}
	}
}

func TestReconnectUsesCurrentPlaylistIDForRepeatedPath(t *testing.T) {
	isolatePlayerCache(t)
	dir, err := os.MkdirTemp("", "plexctl-reconnect-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "ipc")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	media := "https://plex.invalid/repeated.flac?X-Plex-Token=test-token"
	first := PlayItem{URL: media, RatingKey: "first", Title: "First", Duration: 1}
	second := PlayItem{URL: media, RatingKey: "second", Title: "Second", Duration: 2}
	properties := map[string]any{
		"mpv-version":               "queue-test",
		"user-data/plex-queue":      []queueEntry{{ID: 10, PlayItem: first}, {ID: 11, PlayItem: second}},
		"user-data/plex-title":      "First",
		"user-data/plex-rating-key": "first",
		"user-data/plex-no-report":  true,
		"user-data/plex-audio-only": false,
		"path":                      media,
		"user-data/plex-server-id":  "queue-test",
		"user-data/plex-server-url": "https://plex.invalid",
		"playlist":                  []map[string]any{{"id": 10}, {"id": 11, "current": true}},
	}
	peerDone := make(chan struct{})
	go func() {
		defer close(peerDone)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		decoder, encoder := json.NewDecoder(conn), json.NewEncoder(conn)
		for {
			var request struct {
				ID      uint64   `json:"request_id"`
				Command []string `json:"command"`
			}
			if decoder.Decode(&request) != nil {
				return
			}
			var data any
			if len(request.Command) > 1 {
				data = properties[request.Command[1]]
			}
			if encoder.Encode(map[string]any{"request_id": request.ID, "error": "success", "data": data}) != nil {
				return
			}
		}
	}()
	pm := newPlayerManager(socket)
	t.Cleanup(func() { pm.Close(); _ = listener.Close(); <-peerDone })
	if message := pm.Reconnect()(); message != nil {
		t.Fatalf("reconnect: %v", message)
	}
	if status := pm.Status(); status.Key != "second" || status.Title != "Second" || status.Duration != 2 {
		t.Fatalf("reattached to wrong occurrence: key=%q title=%q duration=%g", status.Key, status.Title, status.Duration)
	}
}

package player

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/ygelfand/plexctl/internal/config"
)

func queueRequestConnection(t *testing.T) (*ipcClient, <-chan string) {
	t.Helper()
	left, right := net.Pipe()
	loaded := make(chan string, 16)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { _ = right.Close() }()
		decoder, encoder := json.NewDecoder(right), json.NewEncoder(right)
		for {
			var request struct {
				ID      uint64          `json:"request_id"`
				Command json.RawMessage `json:"command"`
			}
			if decoder.Decode(&request) != nil {
				return
			}
			var data any
			var command []any
			quit := false
			if json.Unmarshal(request.Command, &command) == nil {
				if len(command) > 0 {
					quit = command[0] == "quit"
				}
				if len(command) > 1 && command[0] == "get_property" {
					switch command[1] {
					case "mpv-version":
						data = "queue-test"
					case "playlist":
						data = []map[string]any{{"id": 101, "current": true}}
					}
				}
			} else {
				var named struct {
					Name string `json:"name"`
					URL  string `json:"url"`
				}
				if json.Unmarshal(request.Command, &named) == nil && named.Name == "loadfile" {
					loaded <- named.URL
					if encoder.Encode(terminalEvent{Event: "start-file", PlaylistEntryID: 101}) != nil ||
						encoder.Encode(terminalEvent{Event: "file-loaded"}) != nil {
						return
					}
				}
			}
			if encoder.Encode(map[string]any{"request_id": request.ID, "error": "success", "data": data}) != nil || quit {
				return
			}
		}
	}()
	t.Cleanup(func() { _ = left.Close(); _ = right.Close(); <-done })
	return &ipcClient{conn: left, encoder: json.NewEncoder(left), decoder: json.NewDecoder(left)}, loaded
}

func TestResolveAndPlayCancelsObsoleteQueue(t *testing.T) {
	for _, action := range []string{"stop", "new selection"} {
		t.Run(action, func(t *testing.T) {
			isolatePlayerCache(t)
			// A regression must not launch a real player when the old request wins.
			t.Setenv("PATH", "")
			started, release := make(chan struct{}), make(chan struct{})
			var startedOnce, releaseOnce sync.Once
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/library/metadata/album/children" {
					t.Errorf("unexpected Plex request: %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				startedOnce.Do(func() { close(started) })
				select {
				case <-release:
				case <-r.Context().Done():
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{"ratingKey":"old","type":"track","title":"Old track","Media":[{"Part":[{"key":"/old.flac"}]}]}]}}`))
			}))
			t.Cleanup(server.Close)
			target := config.ServerTarget{ID: "cancel-queue", URL: server.URL, Token: "test-token"}
			pm := newPlayerManager("")
			var loaded <-chan string
			pm.conn, loaded = queueRequestConnection(t)
			pm.ensureReporter()
			oldContext := pm.beginRequest(target.Context(t.Context()))
			key := "album"
			command := pm.resolveAndPlay(oldContext, &components.Metadata{RatingKey: &key, Type: "album"}, true, config.VideoWindow, 0)
			result, finished := make(chan any, 1), make(chan struct{})
			go func() { defer close(finished); result <- command() }()
			t.Cleanup(func() {
				releaseOnce.Do(func() { close(release) })
				pm.Close()
				<-finished
			})
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("album resolution did not start")
			}
			switch action {
			case "stop":
				if message := pm.StopPlayback()(); message != nil {
					t.Fatalf("stop: %v", message)
				}
			case "new selection":
				current := pm.beginRequest(target.Context(t.Context()))
				key := "new"
				track := &components.Metadata{RatingKey: &key, Type: "track", Title: "New track",
					Media: []components.Media{{Part: []components.Part{{Key: "/new.flac"}}}}}
				if message := pm.resolveAndPlay(current, track, true, config.VideoWindow, 0)(); message != nil {
					t.Fatalf("new selection: %v", message)
				}
				if current.Err() != nil || pm.Status().Key != "new" {
					t.Fatal("new selection did not become active")
				}
			}
			if oldContext.Err() != context.Canceled {
				t.Errorf("superseded resolution was not canceled: %v", oldContext.Err())
			}
			releaseOnce.Do(func() { close(release) })
			select {
			case message := <-result:
				if message != nil {
					t.Fatalf("obsolete request produced a user-visible result: %v", message)
				}
			case <-time.After(time.Second):
				t.Fatal("obsolete request did not finish after cancellation")
			}
			if action == "stop" {
				if status := pm.Status(); status.Active() {
					t.Fatal("old queue restarted playback after stop")
				}
			} else if pm.Status().Key != "new" {
				t.Fatal("old queue replaced the newer selection")
			}
			for {
				select {
				case mediaURL := <-loaded:
					if action == "stop" || mediaURL != target.URL+"/new.flac?X-Plex-Token=test-token" {
						t.Errorf("obsolete queue reached mpv: %s", mediaURL)
					}
				default:
					return
				}
			}
		})
	}
}

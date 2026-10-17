package player

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/ygelfand/plexctl/internal/config"
)

func isolatePlayerCache(t *testing.T) {
	t.Helper()
	cfg := config.Get()
	cacheDir, noCache := cfg.CacheDir, cfg.NoCache
	cfg.CacheDir, cfg.NoCache = t.TempDir(), true
	t.Cleanup(func() { cfg.CacheDir, cfg.NoCache = cacheDir, noCache })
}

type timelineReport struct {
	key, state     string
	time, duration int64
}

func readTimeline(r *http.Request) timelineReport {
	query := r.URL.Query()
	position, _ := strconv.ParseInt(query.Get("time"), 10, 64)
	duration, _ := strconv.ParseInt(query.Get("duration"), 10, 64)
	return timelineReport{key: query.Get("ratingKey"), state: query.Get("state"), time: position, duration: duration}
}

func timelineServer(t *testing.T) (config.ServerTarget, <-chan timelineReport) {
	t.Helper()
	reports := make(chan timelineReport, 256)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reports <- readTimeline(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{}}`))
	}))
	t.Cleanup(server.Close)
	return config.ServerTarget{ID: "queue-test", URL: server.URL, Token: "test-token"}, reports
}

func TestMonitorKeepsTrackIdentityWhenBoundaryArrivesDuringSample(t *testing.T) {
	isolatePlayerCache(t)
	server, reports := timelineServer(t)
	left, right := net.Pipe()
	pm := newPlayerManager("")
	first := PlayItem{URL: "first", RatingKey: "one", Duration: 5}
	second := PlayItem{URL: "second", RatingKey: "two", Duration: 9}
	pm.queue = []queueEntry{{ID: 10, PlayItem: first}, {ID: 11, PlayItem: second}}
	status := PlayerStatus{Server: server, State: operations.StatePlaying}
	setTrack(&status, first)
	status.Time = 4.9
	pm.setStatus(status)
	peerDone := make(chan struct{})
	go func() {
		defer close(peerDone)
		defer func() { _ = right.Close() }()
		decoder, encoder := json.NewDecoder(right), json.NewEncoder(right)
		sent := false
		for {
			var request struct {
				ID      uint64 `json:"request_id"`
				Command []any  `json:"command"`
			}
			if decoder.Decode(&request) != nil {
				return
			}
			var data any
			if len(request.Command) > 1 && request.Command[0] == "get_property" {
				switch request.Command[1] {
				case "idle-active", "pause", "mute":
					data = false
				case "time-pos":
					if !sent {
						for _, event := range []terminalEvent{
							{Event: "end-file", PlaylistEntryID: 10, Reason: "eof"},
							{Event: "start-file", PlaylistEntryID: 11},
						} {
							if encoder.Encode(event) != nil {
								return
							}
						}
						sent = true
					}
					data = 2.0
				case "duration":
					data = 9.0
				case "volume":
					data = 100.0
				}
			}
			if encoder.Encode(map[string]any{"request_id": request.ID, "error": "success", "data": data}) != nil {
				return
			}
		}
	}()
	pm.attach(&ipcClient{conn: left, encoder: json.NewEncoder(left), decoder: json.NewDecoder(left)})
	t.Cleanup(func() { pm.Close(); _ = right.Close(); <-peerDone })
	select {
	case report := <-reports:
		if report.key != "one" || report.state != "stopped" || report.time != 5000 || report.duration != 5000 {
			t.Fatalf("next track's sample changed the previous completion: %+v", report)
		}
	case <-time.After(time.Second):
		t.Fatal("missing first track completion")
	}
	if got := pm.Status(); got.Key != "two" || got.Duration != 9 {
		t.Fatalf("new track identity was not applied: key=%q duration=%g", got.Key, got.Duration)
	}
}

func TestStopPlaybackOrdersFinalReportAfterPendingProgress(t *testing.T) {
	isolatePlayerCache(t)
	started, release := make(chan struct{}), make(chan struct{})
	stopped := make(chan struct{}, 8)
	var first, releaseOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")
		if state == "playing" {
			first.Do(func() { close(started); <-release })
		}
		if state == "stopped" {
			stopped <- struct{}{}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{}}`))
	}))
	t.Cleanup(server.Close)
	pm := newPlayerManager("")
	status := PlayerStatus{Server: config.ServerTarget{ID: "ordering-test", URL: server.URL, Token: "test-token"},
		Key: "one", State: operations.StatePlaying}
	pm.setStatus(status)
	pm.ensureReporter()
	pm.ioMu.Lock()
	pm.enqueueReports([]PlayerStatus{status})
	pm.ioMu.Unlock()
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(func() { unblock(); pm.Close() })
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("progress report did not start")
	}
	if message := pm.StopPlayback()(); message != nil {
		t.Fatalf("stop: %v", message)
	}
	select {
	case <-stopped:
		t.Fatal("stopped reached Plex while an earlier playing report was still in flight")
	case <-time.After(50 * time.Millisecond):
	}
	unblock()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("stop report was lost after the earlier request completed")
	}
}

func TestStopPlaybackAfterClose(t *testing.T) {
	pm := newPlayerManager("")
	pm.setStatus(PlayerStatus{Key: "one", State: operations.StatePlaying, NoReport: true})
	pm.ensureReporter()
	pm.Close()
	if message := pm.StopPlayback()(); message != nil {
		t.Fatalf("stop after close: %v", message)
	}
}

func TestCloseDeadlineAppliesWhenReportQueueIsFull(t *testing.T) {
	isolatePlayerCache(t)
	started, release := make(chan struct{}), make(chan struct{})
	var first sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		first.Do(func() { close(started) })
		select {
		case <-release:
		case <-r.Context().Done():
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{}}`))
	}))
	t.Cleanup(server.Close)
	pm := newPlayerManager("")
	pm.ensureReporter()
	var producerDone, closed chan struct{}
	t.Cleanup(func() {
		close(release)
		if closed != nil {
			<-closed
		} else {
			pm.Close()
		}
		if producerDone != nil {
			<-producerDone
		}
	})
	slow := PlayerStatus{Server: config.ServerTarget{ID: "shutdown-test", URL: server.URL, Token: "test-token"},
		Key: "one", State: operations.StatePlaying}
	pm.ioMu.Lock()
	pm.enqueueReports([]PlayerStatus{slow, slow})
	pm.ioMu.Unlock()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timeline request never started")
	}
	pm.ioMu.Lock()
	for range cap(pm.reports) {
		pm.enqueueReports([]PlayerStatus{{NoReport: true}})
	}
	pm.ioMu.Unlock()
	held := make(chan struct{})
	producerDone, closed = make(chan struct{}), make(chan struct{})
	go func() {
		defer close(producerDone)
		pm.ioMu.Lock()
		defer pm.ioMu.Unlock()
		close(held)
		pm.enqueueReports([]PlayerStatus{{NoReport: true}})
	}()
	<-held
	go func() { defer close(closed); pm.Close() }()
	select {
	case <-closed:
	case <-time.After(4500 * time.Millisecond):
		t.Fatal("shutdown deadline did not cover waiting for a full report queue")
	}
}

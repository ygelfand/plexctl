package player

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/ygelfand/plexctl/internal/config"
)

func TestPlaybackRequestOutlivesMetadataDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pm := newPlayerManager("")
		defer pm.Close()
		server := config.ServerTarget{URL: "https://plex.invalid", Token: "test-token"}
		ctx := pm.beginRequest(server.Context(context.Background()))
		metadata := &components.Metadata{Type: "movie",
			Media: []components.Media{{Part: []components.Part{{Key: "/video"}}}}}
		if _, err := BuildPlayRequest(ctx, metadata, true); err != nil {
			t.Fatal(err)
		}
		// Foreground mpv inherits this context after metadata resolution.
		time.Sleep(3 * time.Hour)
		if err := ctx.Err(); err != nil {
			t.Fatalf("playback expired without a stop or new selection: %v", err)
		}
		next := pm.beginRequest(context.Background())
		if ctx.Err() != context.Canceled {
			t.Fatal("a new selection did not cancel foreground playback")
		}
		pm.Close()
		if next.Err() != context.Canceled {
			t.Fatal("closing the manager did not cancel foreground playback")
		}
	})
}

func TestQueueEventsReportEveryTrack(t *testing.T) {
	server := config.ServerTarget{ID: "friend", URL: "https://friend.invalid", Token: "private"}
	first := PlayItem{URL: "repeated-file", RatingKey: "one", Title: "First", Duration: 3}
	second := PlayItem{URL: "repeated-file", RatingKey: "two", Title: "Second", Duration: 4}
	pm := newPlayerManager("")
	pm.queue = []queueEntry{{ID: 10, PlayItem: first}, {ID: 11, PlayItem: second}}
	pm.conn = &ipcClient{events: []terminalEvent{
		{Event: "end-file", PlaylistEntryID: 10, Reason: "eof"},
		{Event: "start-file", PlaylistEntryID: 11},
		{Event: "end-file", PlaylistEntryID: 11, Reason: "eof"},
		{Event: "end-file", PlaylistEntryID: 999, Reason: "eof"},
	}}
	status := PlayerStatus{Server: server, AudioOnly: true, NoReport: true, State: operations.StatePlaying}
	setTrack(&status, first)
	reports := pm.queueEvents(&status)
	if len(reports) != 3 {
		t.Fatalf("missing track boundaries: %+v", reports)
	}
	for i, report := range reports {
		if report.Server != server || !report.NoReport || !report.AudioOnly {
			t.Fatalf("track %d lost playback context: %+v", i, report)
		}
	}
	if reports[0].Key != "one" || reports[0].Time != 3 || reports[0].State != operations.StateStopped {
		t.Fatalf("first track not completed: %+v", reports[0])
	}
	if reports[1].Key != "two" || reports[1].Time != 0 || reports[1].Title != "Second" {
		t.Fatalf("second track not reset: %+v", reports[1])
	}
	if status.Key != "two" || status.Time != 4 || status.State != operations.StateStopped {
		t.Fatalf("last track not completed: %+v", status)
	}
	if len(pm.queueEvents(&status)) != 0 {
		t.Fatal("track boundaries were reported twice")
	}
}

package plex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/ygelfand/plexctl/internal/config"
)

func TestResolveMusicPlayback(t *testing.T) {
	t.Setenv("PLEXCTL_TOKEN", "music-test-token")
	track := components.Metadata{RatingKey: ptr("track"), Type: "track", Title: "Song", Media: []components.Media{
		{}, // Plex can list an unavailable version before a playable one.
		{Part: []components.Part{{}, {Key: "/music.flac"}}},
	}}
	artist := components.Metadata{RatingKey: ptr("artist"), Type: "artist"}
	album := components.Metadata{RatingKey: ptr("album"), Type: "album"}
	empty := components.Metadata{RatingKey: ptr("empty"), Type: "album"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("X-Plex-Token") != "music-test-token" && r.Header.Get("X-Plex-Token") != "music-test-token" {
			t.Error("music request did not use the configured token")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		var items []components.Metadata
		switch r.URL.Path {
		case "/library/metadata/artist/children":
			items = []components.Metadata{empty, album}
		case "/library/metadata/album/children":
			// Library listings need not include the track's Media/Part.
			items = []components.Metadata{{RatingKey: ptr("track"), Type: "track", Title: "Song"}}
		case "/library/metadata/track":
			items = []components.Metadata{track}
		case "/library/metadata/empty/children":
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": map[string]any{"Metadata": items}})
	}))
	defer server.Close()
	cfg := config.Get()
	old := *cfg
	t.Cleanup(func() { *cfg = old })
	cfg.DefaultServer = "music-test"
	cfg.Servers = map[string]config.Server{"music-test": {URL: server.URL}}
	cfg.CacheDir = t.TempDir()
	cfg.NoCache = true

	for _, meta := range []components.Metadata{artist, album, track} {
		t.Run(meta.Type, func(t *testing.T) {
			got, err := ResolvePlayable(context.Background(), &meta)
			if err != nil {
				t.Fatal(err)
			}
			if got.RatingKey == nil || *got.RatingKey != "track" || PlayablePart(got).Key != "/music.flac" {
				t.Fatalf("did not resolve the playable track: %+v", got)
			}
		})
	}
	if _, err := ResolvePlayable(context.Background(), &empty); err == nil || !strings.Contains(err.Error(), "no playable tracks") {
		t.Fatalf("empty album should have an actionable error, got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ResolvePlayable(ctx, &artist); err == nil {
		t.Fatal("canceled traversal succeeded")
	}
}

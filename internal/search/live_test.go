//go:build integration

package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/ygelfand/plexctl/internal/config"
)

func TestMusicSearchAndIndexIncludeEveryPage(t *testing.T) {
	counts := map[string]int{"": 137, "9": 113, "10": 205}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var container map[string]any
		switch r.URL.Path {
		case "/library/sections/all":
			container = map[string]any{"Directory": []any{map[string]any{"key": "3", "title": "Music", "type": "artist"}}}
		case "/library/sections/3/all":
			typ := r.URL.Query().Get("type")
			startText := r.Header.Get("X-Plex-Container-Start")
			if startText == "" {
				startText = r.URL.Query().Get("X-Plex-Container-Start")
			}
			start, _ := strconv.Atoi(startText)
			count, ok := counts[typ]
			if !ok {
				t.Errorf("wrong Plex music type: %q", typ)
			}
			// Deliberately return smaller pages than requested.
			items := []any{}
			for i := start; i < min(start+37, count); i++ {
				items = append(items, map[string]any{
					"ratingKey": fmt.Sprintf("%s-%d", typ, i), "type": map[string]string{"": "artist", "9": "album", "10": "track"}[typ],
					"title": fmt.Sprintf("Title %d", i), "parentTitle": "Album Name", "grandparentTitle": "Artist Name",
				})
			}
			container = map[string]any{"Metadata": items, "totalSize": count}
		case "/hubs/search":
			if r.URL.Query().Get("query") != "hidden & finale" {
				t.Error("query was not encoded correctly")
			}
			container = map[string]any{"Hub": []any{map[string]any{"type": "track", "Metadata": []any{map[string]any{
				"ratingKey": "hidden", "title": "Hidden Finale", "type": "track", "parentTitle": "Album Name",
				"grandparentTitle": "Artist Name", "librarySectionID": 3,
			}}}}}
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": container})
	}))
	defer server.Close()
	cfg := config.Get()
	old := *cfg
	t.Cleanup(func() { *cfg = old })
	t.Setenv("PLEXCTL_TOKEN", "qa")
	cfg.DefaultServer, cfg.CacheDir = "search-test", t.TempDir()
	cfg.Servers = map[string]config.Server{"search-test": {URL: server.URL}}

	entries, err := Live(context.Background(), "hidden & finale")
	if err != nil || len(entries) != 1 || entries[0].SectionID != "3" || entries[0].Artist != "Artist Name" {
		t.Fatalf("live music search: %+v, %v", entries, err)
	}
	var index SearchIndex
	if err := index.Reindex(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	entries, indexedAt := index.Snapshot()
	if len(entries) != 455 || indexedAt.IsZero() {
		t.Fatalf("index has %d entries; want all 455 artists, albums, and tracks", len(entries))
	}
	last := entries[len(entries)-1]
	if last.Type != "track" || !strings.Contains(last.SearchableText(), "Artist Name Album Name") {
		t.Fatalf("track cannot be found by artist/album: %+v", last)
	}
}

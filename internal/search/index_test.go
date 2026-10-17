package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ygelfand/plexctl/internal/cache"
	"github.com/ygelfand/plexctl/internal/config"
)

func configureIndexTest(t *testing.T, url string) *config.Config {
	t.Helper()
	cfg := config.Get()
	old := *cfg
	t.Cleanup(func() { *cfg = old })
	t.Setenv("PLEXCTL_TOKEN", "")
	cfg.DefaultServer, cfg.CacheDir, cfg.NoCache = "index-test", t.TempDir(), false
	cfg.Servers = map[string]config.Server{"index-test": {URL: url}}
	cfg.HomeUser = config.HomeUser{AuthToken: "original-user", AccessToken: "original-user", ServerID: "index-test"}
	return cfg
}

func TestIndexRetainsItsProfileForChildrenAndPersistence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token := r.Header.Get("X-Plex-Token"); token != "original-user" {
			t.Errorf("request %s used another profile's token: %q", r.URL.Path, token)
		}
		var container any
		switch r.URL.Path {
		case "/library/sections/all":
			container = map[string]any{"Directory": []any{map[string]any{"key": "1", "title": "Shows", "type": "show"}}}
		case "/library/sections/1/all":
			container = map[string]any{"totalSize": 1, "Metadata": []any{map[string]any{"ratingKey": "show", "title": "Private show", "type": "show"}}}
		case "/library/metadata/show/children":
			container = map[string]any{"totalSize": 1, "Metadata": []any{map[string]any{"ratingKey": "season", "title": "Season", "type": "season"}}}
		case "/library/metadata/season/children":
			container = map[string]any{"totalSize": 1, "Metadata": []any{map[string]any{"ratingKey": "episode", "title": "Episode", "type": "episode"}}}
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": container})
	}))
	defer server.Close()
	cfg := configureIndexTest(t, server.URL)
	original := cfg.ServerTarget()
	idx := GetIndex()
	// The reindex command may be scheduled after the user has switched profiles.
	cfg.HomeUser = config.HomeUser{AuthToken: "new-user", AccessToken: "new-user", ServerID: "index-test"}
	if err := idx.Reindex(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	cm, err := cache.Get(cfg.CacheDir)
	if err != nil {
		t.Fatal(err)
	}
	var saved SearchIndex
	if err := cm.Get(original.CacheNamespace()+"/search_index", &saved); err != nil || len(saved.Entries) != 3 {
		t.Fatalf("original profile index was not saved: entries=%d err=%v", len(saved.Entries), err)
	}
	if err := cm.Get(cfg.MediaCacheNamespace()+"/search_index", &saved); err == nil {
		t.Fatal("private search results were written to the new profile's cache")
	}
}

func TestIndexFailurePreservesCompletedSnapshot(t *testing.T) {
	for _, failure := range []string{"season without rating key", "children request fails", "missing library container"} {
		t.Run(failure, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/library/sections/all":
					_, _ = fmt.Fprint(w, `{"MediaContainer":{"Directory":[{"key":"1","title":"Shows","type":"show"}]}}`)
				case "/library/sections/1/all":
					if failure == "missing library container" {
						_, _ = fmt.Fprint(w, `{}`)
						return
					}
					_, _ = fmt.Fprint(w, `{"MediaContainer":{"totalSize":1,"Metadata":[{"ratingKey":"show","title":"Show","type":"show"}]}}`)
				case "/library/metadata/show/children":
					if failure == "children request fails" {
						w.WriteHeader(http.StatusForbidden)
						return
					}
					_, _ = fmt.Fprint(w, `{"MediaContainer":{"totalSize":1,"Metadata":[{"title":"Season","type":"season"}]}}`)
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
				}
			}))
			defer server.Close()
			configureIndexTest(t, server.URL)
			indexedAt := time.Now().Add(-time.Hour)
			idx := SearchIndex{Entries: []IndexEntry{{RatingKey: "saved"}}, LastIndexed: indexedAt}
			if err := idx.Reindex(context.Background(), nil); err == nil {
				t.Fatal("incomplete indexing succeeded")
			}
			entries, gotTime := idx.Snapshot()
			if len(entries) != 1 || entries[0].RatingKey != "saved" || !gotTime.Equal(indexedAt) {
				t.Fatal("failed indexing replaced the last completed snapshot")
			}
		})
	}
}

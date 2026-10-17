package plex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/ygelfand/plexctl/internal/config"
)

func TestArtistListingsAndQueueIncludeCompilations(t *testing.T) {
	cfg := config.Get()
	previous := cfg.CacheDir
	cfg.CacheDir = t.TempDir()
	t.Cleanup(func() { cfg.CacheDir = previous })
	artist := components.Metadata{RatingKey: ptr("artist"), Type: "artist",
		AdditionalProperties: map[string]any{"librarySectionID": float64(7)}}
	track := func(key string) components.Metadata {
		return components.Metadata{RatingKey: ptr(key), Type: "track", Title: key,
			Media: []components.Media{{Part: []components.Part{{Key: "/" + key + ".flac"}}}}}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != "friend & token" || r.URL.Query().Has("X-Plex-Token") {
			t.Error("listing did not keep the originating credential in a header")
		}
		w.Header().Set("Content-Type", "application/json")
		items := []components.Metadata{}
		total := 3
		switch r.URL.Path {
		case "/library/sections/7/all":
			if r.URL.Query().Get("artist.id") != "artist" {
				t.Error("artist filter missing")
			}
			switch r.URL.Query().Get("type") {
			case "9":
				items = []components.Metadata{{Title: "Soundtrack", Type: "album"}}
				total = 1
			case "10":
				switch r.Header.Get("X-Plex-Container-Start") {
				case "0":
					items = []components.Metadata{track("own"), {RatingKey: ptr("compilation"), Type: "track"}}
				case "2":
					items = []components.Metadata{track("guest")}
				default:
					t.Error("wrong page offset")
				}
			default:
				t.Error("wrong media type")
			}
		case "/library/metadata/compilation":
			items = []components.Metadata{track("compilation")}
		default:
			t.Errorf("unexpected children-only lookup: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": map[string]any{
			"Metadata": items, "totalSize": total,
		}})
	}))
	defer server.Close()
	ctx := (config.ServerTarget{ID: "compilations", URL: server.URL, Token: "friend & token"}).Context(t.Context())
	albums, err := GetMusicChildren(ctx, &artist)
	if err != nil || len(albums) != 1 || albums[0].Title != "Soundtrack" {
		t.Fatalf("compilation albums: %v, %v", albums, err)
	}
	tracks, err := CollectTracks(ctx, &artist)
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	for _, item := range tracks {
		keys = append(keys, *item.RatingKey)
		if PlayablePart(&item) == nil {
			t.Fatal("sparse metadata was not resolved")
		}
	}
	if !reflect.DeepEqual(keys, []string{"own", "compilation", "guest"}) {
		t.Fatalf("incomplete or reordered queue: %v", keys)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := CollectTracks(canceled, &artist); err == nil {
		t.Fatal("canceled queue lookup succeeded")
	}
}

func TestCollectTracksRejectsPartialQueues(t *testing.T) {
	cfg := config.Get()
	previous := cfg.CacheDir
	cfg.CacheDir = t.TempDir()
	t.Cleanup(func() { cfg.CacheDir = previous })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/library/metadata/album/children" {
			_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{"ratingKey":"missing","type":"track"}]}}`))
			return
		}
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	ctx := (config.ServerTarget{ID: "partial-queue", URL: server.URL, Token: "test"}).Context(t.Context())
	_, err := CollectTracks(ctx, &components.Metadata{RatingKey: ptr("album"), Type: "album"})
	if err == nil {
		t.Fatal("failed track lookup was silently skipped")
	}
}

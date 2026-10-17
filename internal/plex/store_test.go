package plex

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ygelfand/plexctl/internal/config"
)

func TestStoreSeparatesProfilesAndRetainsRequestCredentials(t *testing.T) {
	t.Setenv("PLEXCTL_TOKEN", "")
	cfg := config.Get()
	old := *cfg
	t.Cleanup(func() { *cfg = old })
	cfg.CacheDir = t.TempDir()
	cfg.NoCache = false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		token := r.Header.Get("X-Plex-Token")
		container := map[string]any{}
		switch r.URL.Path {
		case "/api/users":
			container["User"] = []map[string]any{{"id": 1, "title": token}}
		case "/library/sections/all":
			container["Directory"] = []map[string]any{{"key": "1", "title": token, "type": "movie"}}
		case "/library/metadata/1":
			container["Metadata"] = []map[string]any{{"ratingKey": "1", "title": token, "type": "movie"}}
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": container})
	}))
	defer server.Close()
	// The SDK fixes account lookups to plex.tv; redirect every transport request
	// to our local fixture so no test can contact an actual account endpoint.
	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = transport })
	http.DefaultTransport = resourceTransport(func(req *http.Request) (*http.Response, error) {
		local := req.Clone(req.Context())
		local.URL.Scheme, local.URL.Host = endpoint.Scheme, endpoint.Host
		return transport.RoundTrip(local)
	})
	cfg.Token = "owner"
	cfg.HomeUser = config.HomeUser{}
	cfg.DefaultServer = "profile-store-test"
	cfg.Servers = map[string]config.Server{cfg.DefaultServer: {
		URL: server.URL, AccessToken: "owner-resource", TokenSource: cfg.TokenSource(),
	}}
	ownerContext := cfg.ServerTarget().Context(t.Context())
	owner, err := GetStore()
	if err != nil {
		t.Fatal(err)
	}
	checkStore := func(store *Store, wantAccount, wantServer string) {
		t.Helper()
		user, err := store.ResolveUser(t.Context(), 1)
		if err != nil || user != wantAccount {
			t.Fatalf("user = %q, %v; want %q", user, err, wantAccount)
		}
		library, err := store.ResolveLibrary(t.Context(), "1")
		if err != nil || library != wantServer {
			t.Fatalf("library = %q, %v; want %q", library, err, wantServer)
		}
		meta, err := store.GetMetadata(t.Context(), "1", false)
		if err != nil {
			t.Fatal(err)
		}
		if meta.Title != wantServer {
			t.Fatalf("metadata from wrong user: %q, want %q", meta.Title, wantServer)
		}
	}
	checkStore(owner, "owner", "owner-resource")
	cfg.HomeUser = config.HomeUser{AuthToken: "restricted", AccessToken: "restricted-resource", ServerID: cfg.DefaultServer}
	restricted, err := GetStore()
	if err != nil {
		t.Fatal(err)
	}
	checkStore(restricted, "restricted", "restricted-resource")
	retained, err := GetStoreForContext(ownerContext)
	if err != nil {
		t.Fatal(err)
	}
	checkStore(retained, "owner", "owner-resource")
}

func TestStoreInitializationCanRetryAndRejectsEmptyMetadata(t *testing.T) {
	t.Setenv("PLEXCTL_TOKEN", "")
	cfg := config.Get()
	old := *cfg
	t.Cleanup(func() { *cfg = old })
	cfg.CacheDir = t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	target := config.ServerTarget{ID: "retry-store", URL: server.URL}
	if _, err := GetStoreForContext(target.Context(t.Context())); err == nil {
		t.Fatal("missing credentials unexpectedly succeeded")
	}
	target.Token = "valid"
	store, err := GetStoreForContext(target.Context(t.Context()))
	if err != nil || store == nil {
		t.Fatalf("store could not recover after configuration was fixed: %v", err)
	}
	if _, err := store.GetMetadata(t.Context(), "1", true); err == nil {
		t.Fatal("missing MediaContainer unexpectedly succeeded")
	}
}

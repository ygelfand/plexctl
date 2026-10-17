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

type resourceTransport func(*http.Request) (*http.Response, error)

func (f resourceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDiscoveryUsesCapturedAccountTokenAndIncludesFriends(t *testing.T) {
	t.Setenv("PLEXCTL_TOKEN", "")
	cfg := config.Get()
	old := *cfg
	oldTransport := http.DefaultTransport
	t.Cleanup(func() { *cfg = old; http.DefaultTransport = oldTransport })
	cfg.Token = "account"
	cfg.HomeUser = config.HomeUser{AuthToken: "profile-account", AccessToken: "server-only"}
	ctx := cfg.ServerTarget().Context(t.Context())
	cfg.HomeUser = config.HomeUser{AuthToken: "new-profile", AccessToken: "new-server-token"}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != "profile-account" {
			t.Error("discovery used a server token")
		}
		if r.URL.Query().Get("includeRelay") != "1" {
			t.Error("relay discovery was disabled")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]components.PlexDevice{
			{Name: "Friend", ClientIdentifier: "friend", Provides: "server", AccessToken: "shared"},
			{Name: "Mine", ClientIdentifier: "mine", Provides: "server", Owned: true},
			{Name: "Player", Provides: "player"},
		})
	}))
	defer fixture.Close()
	http.DefaultTransport = resourceTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "plex.tv" {
			t.Fatalf("unexpected discovery host %s", r.URL.Host)
		}
		clone := r.Clone(r.Context())
		clone.URL.Scheme, clone.URL.Host = "http", strings.TrimPrefix(fixture.URL, "http://")
		return oldTransport.RoundTrip(clone)
	})
	servers, err := DiscoverServers(ctx)
	if err != nil || len(servers) != 2 || !servers[0].Owned || servers[1].AccessToken != "shared" {
		t.Fatalf("discover owned and shared servers: count=%d, error=%v", len(servers), err)
	}
}

func TestConnectSharedServerFallsBackToRelayWithoutAccountToken(t *testing.T) {
	var attempts []string
	makeServer := func(name, identity string, status int) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts = append(attempts, name)
			wantToken := "resource-token"
			if r.URL.Path == "/identity" {
				wantToken = ""
			}
			if r.Header.Get("X-Plex-Token") != wantToken {
				t.Error("wrong shared-server credentials")
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": map[string]any{"machineIdentifier": identity}})
		}))
	}
	wrong := makeServer("wrong", "another-server", 200)
	defer wrong.Close()
	denied := makeServer("denied", "friend", 401)
	defer denied.Close()
	relay := makeServer("relay", "friend", 200)
	defer relay.Close()
	device := components.PlexDevice{Name: "Friend", ClientIdentifier: "friend", AccessToken: "resource-token", Connections: []components.Connections{
		{URI: relay.URL, Relay: true}, {URI: wrong.URL}, {URI: denied.URL},
	}}
	target, err := ConnectServer(context.Background(), device)
	if err != nil || target.URL != relay.URL || !target.Shared {
		t.Fatalf("relay fallback: %v", err)
	}
	if strings.Join(attempts, ",") != "wrong,denied,relay,relay" {
		t.Fatalf("unexpected connection attempts: %v", attempts)
	}
	device.AccessToken = ""
	if _, err := ConnectServer(context.Background(), device); err == nil {
		t.Fatal("accepted missing share token")
	}
	device.AccessToken, device.HTTPSRequired = "resource-token", true
	if _, err := ConnectServer(context.Background(), device); err == nil {
		t.Fatal("downgraded a secure-only server")
	}
}

func TestSharedMetadataAndArtworkDoNotChangeOrReuseOwnedServer(t *testing.T) {
	t.Setenv("PLEXCTL_TOKEN", "")
	cfg := config.Get()
	old := *cfg
	t.Cleanup(func() { *cfg = old })
	cfg.Token, cfg.HomeUser = "own-token", config.HomeUser{}
	cfg.DefaultServer, cfg.Servers, cfg.NoCache = "owned-isolation", map[string]config.Server{"owned-isolation": {URL: "https://owned.invalid"}}, false
	cfg.CacheDir = t.TempDir()
	newServer := func(title, token string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := r.Header.Get("X-Plex-Token")
			if got == "" {
				got = r.URL.Query().Get("X-Plex-Token")
			}
			if got != token {
				t.Error("request used another server's token")
				http.Error(w, "denied", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/art" {
				_, _ = w.Write([]byte(title))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": map[string]any{"Metadata": []any{map[string]any{"ratingKey": "1", "title": title, "type": "movie"}}}})
		}))
	}
	a, b := newServer("First friend", "first+&token"), newServer("Second friend", "second-token")
	defer a.Close()
	defer b.Close()
	for _, target := range []config.ServerTarget{
		{ID: "shared-a-isolation", URL: a.URL, Token: "first+&token", Name: "First friend"},
		{ID: "shared-b-isolation", URL: b.URL, Token: "second-token", Name: "Second friend"},
	} {
		ctx := target.Context(context.Background())
		meta, err := GetMetadata(ctx, "1", false)
		if err != nil || meta.Title != target.Name {
			t.Fatalf("metadata crossed servers: %v", err)
		}
		children, err := GetChildren(ctx, "1")
		if err != nil || len(children) != 1 || children[0].Title != target.Name {
			t.Fatalf("children crossed servers: %v", err)
		}
		image, err := GetImage(ctx, "/art")
		if err != nil || string(image) != target.Name {
			t.Fatalf("artwork crossed servers: %v", err)
		}
	}
	if cfg.DefaultServer != "owned-isolation" || cfg.ServerToken() != "own-token" {
		t.Fatal("shared requests changed the owned server")
	}
}

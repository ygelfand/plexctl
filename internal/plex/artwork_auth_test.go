package plex

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ygelfand/plexctl/internal/config"
)

func TestArtworkUsesHomeUserToken(t *testing.T) {
	t.Setenv("PLEXCTL_TOKEN", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != "home+user&token" || r.URL.Query().Has("X-Plex-Token") {
			http.Error(w, "wrong credentials", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte("album artwork"))
	}))
	defer server.Close()
	cfg := config.Get()
	old := *cfg
	t.Cleanup(func() { *cfg = old })
	cfg.Token, cfg.HomeUser.AccessToken = "account-token", "home+user&token"
	cfg.DefaultServer, cfg.CacheDir = "artwork-token-test", t.TempDir()
	cfg.Servers = map[string]config.Server{"artwork-token-test": {URL: server.URL}}
	data, err := GetImage(context.Background(), "/album.png?width=720")
	if err != nil || string(data) != "album artwork" {
		t.Fatalf("Home user artwork failed: %v", err)
	}
}

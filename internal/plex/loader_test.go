package plex

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ygelfand/plexctl/internal/config"
)

func TestLoadDataUsesCapturedServer(t *testing.T) {
	t.Setenv("PLEXCTL_TOKEN", "")
	cfg := config.Get()
	old := *cfg
	t.Cleanup(func() { *cfg = old })
	cfg.CacheDir = t.TempDir()
	cfg.Token = "different-profile"
	cfg.HomeUser = config.HomeUser{}
	cfg.DefaultServer = "different-server"
	cfg.Servers = map[string]config.Server{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != "captured-token" {
			t.Error("loader used credentials from a different profile")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[]}}`))
	}))
	defer server.Close()
	target := config.ServerTarget{ID: "captured-server", URL: server.URL, Token: "captured-token"}
	updates := make(chan any, 10)
	LoadData(target.Context(t.Context()), updates)
	close(updates)
	found := false
	for update := range updates {
		switch result := update.(type) {
		case error:
			t.Fatal(result)
		case LoaderResult:
			found = true
			if result.ServerID != target.ID {
				t.Fatalf("result belongs to wrong server: %q", result.ServerID)
			}
		}
	}
	if !found {
		t.Fatal("loader never completed")
	}
}

func TestLoadDataCancellationUnblocksUpdates(t *testing.T) {
	for _, tt := range []struct {
		name    string
		updates chan any
	}{
		{"no updates channel", nil},
		{"no receiver", make(chan any)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel((config.ServerTarget{}).Context(t.Context()))
			cancel()
			done := make(chan struct{})
			go func() {
				defer close(done)
				LoadData(ctx, tt.updates)
			}()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("canceled loader blocked trying to report an update")
			}
		})
	}
}

package plex

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ygelfand/plexctl/internal/config"
)

func TestGetMetadataRejectsMissingContainer(t *testing.T) {
	cfg := config.Get()
	old := *cfg
	t.Cleanup(func() { *cfg = old })
	cfg.CacheDir, cfg.NoCache = t.TempDir(), true
	for _, test := range []struct{ name, body string }{
		{"omitted", `{}`},
		{"null", `{"MediaContainer":null}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			target := config.ServerTarget{ID: "missing-container", URL: server.URL, Token: "token"}
			if _, err := GetMetadata(target.Context(context.Background()), "1", true); err == nil {
				t.Fatal("missing metadata container was accepted")
			}
		})
	}
}

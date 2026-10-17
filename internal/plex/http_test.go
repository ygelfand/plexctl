package plex

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ygelfand/plexctl/internal/config"
)

func TestMediaURLRejectsOriginChanges(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, path string
	}{
		{"userinfo injection", "@attacker.invalid/stolen"},
		{"absolute URL", "https://attacker.invalid/stolen"},
		{"network path", "//attacker.invalid/stolen"},
		{"relative path", "library/parts/1/file"},
		{"fragment", "/library/parts/1/file#fragment"},
		{"invalid query", "/library/parts/1/file?broken=%xy"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := config.ServerTarget{URL: "https://plex.invalid", Token: "private-token"}
			if _, err := MediaURL(server, test.path); err == nil {
				t.Fatal("unsafe media path was accepted")
			}
		})
	}
}

func TestMediaURLPreservesPathAndUsesOnlyTheSelectedToken(t *testing.T) {
	t.Parallel()
	server := config.ServerTarget{URL: "https://plex.invalid/prefix/", Token: "private+&token"}
	got, err := MediaURL(server, "/library/parts/one%2Ftwo/file?width=720&X-Plex-Token=spoof&x-plex-token=also-spoof")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "plex.invalid" || u.EscapedPath() != "/prefix/library/parts/one%2Ftwo/file" ||
		u.Query().Get("width") != "720" || u.Query().Get("X-Plex-Token") != server.Token ||
		u.Query().Has("x-plex-token") || len(u.Query()["X-Plex-Token"]) != 1 {
		t.Fatal("media URL changed its origin, path, query or credential scope")
	}
}

func TestMediaRequestsRefuseCrossOriginRedirects(t *testing.T) {
	cfg := config.Get()
	old := *cfg
	t.Cleanup(func() { *cfg = old })
	cfg.CacheDir, cfg.NoCache = t.TempDir(), true
	var leaked atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked.Store(true)
		_, _ = io.WriteString(w, `{"MediaContainer":{"Metadata":[]}}`)
	}))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != "redirect-token" {
			t.Error("original request did not use the selected server token")
		}
		if r.URL.Query().Has("X-Plex-Token") {
			t.Error("nonstream request put its credential in the URL")
		}
		http.Redirect(w, r, destination.URL+"/stolen", http.StatusFound)
	}))
	defer origin.Close()
	target := config.ServerTarget{ID: "redirect-test", URL: origin.URL, Token: "redirect-token"}
	ctx := target.Context(context.Background())
	if _, err := GetMetadata(ctx, "1", true); err == nil {
		t.Error("metadata accepted a cross-origin redirect")
	}
	if _, err := GetChildren(ctx, "1"); err == nil {
		t.Error("listing accepted a cross-origin redirect")
	}
	if _, err := GetImage(ctx, "/poster"); err == nil {
		t.Error("artwork accepted a cross-origin redirect")
	}
	if leaked.Load() {
		t.Fatal("authenticated request reached the redirect target")
	}
}

func TestGetImageAllowsSameOriginRedirects(t *testing.T) {
	cfg := config.Get()
	old := *cfg
	t.Cleanup(func() { *cfg = old })
	cfg.CacheDir, cfg.NoCache = t.TempDir(), true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/poster" {
			http.Redirect(w, r, "/image", http.StatusFound)
			return
		}
		if r.Header.Get("X-Plex-Token") != "art-token" {
			t.Error("same-origin redirect lost authentication")
		}
		_, _ = io.WriteString(w, "artwork")
	}))
	defer server.Close()
	target := config.ServerTarget{ID: "same-origin", URL: server.URL, Token: "art-token"}
	got, err := GetImage(target.Context(context.Background()), "/poster")
	if err != nil || string(got) != "artwork" {
		t.Fatalf("same-origin image redirect failed: %v", err)
	}
}

func TestGetImageRejectsOversizedResponses(t *testing.T) {
	cfg := config.Get()
	old := *cfg
	t.Cleanup(func() { *cfg = old })
	cfg.CacheDir, cfg.NoCache = t.TempDir(), true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := strings.Repeat("x", 8192)
		for range 2049 {
			if _, err := io.WriteString(w, chunk); err != nil {
				return
			}
		}
	}))
	defer server.Close()
	target := config.ServerTarget{ID: "oversized-art", URL: server.URL, Token: "art-token"}
	if _, err := GetImage(target.Context(context.Background()), "/poster"); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("expected oversized response rejection, got %v", err)
	}
}

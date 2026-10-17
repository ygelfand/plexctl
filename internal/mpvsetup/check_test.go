package mpvsetup

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestNeedsInstall(t *testing.T) {
	for _, tt := range []struct {
		name, installed, latest string
		want                    bool
	}{
		{"missing", "", "0.41.0", true},
		{"missing offline", "", "", true},
		{"old", "0.37.0", "0.41.0", true},
		{"current", "0.41.0", "0.41.0", false},
		{"newer development build", "0.41.0-1150-g88eb9f2e7", "0.41.0", false},
		{"old development build", "0.40.0-100-gabcdef", "0.41.0", true},
		{"release candidate", "0.41.0-rc1", "0.41.0", true},
		{"newer release candidate", "0.42.0-rc1", "0.41.0", false},
		{"numeric ordering", "0.9.0", "0.41.0", true},
		{"future major", "1.0.0", "0.41.0", false},
		{"offline installed", "0.37.0", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := Status{Installed: tt.installed, Latest: tt.latest}
			if tt.installed != "" {
				s.Path = "/usr/bin/mpv"
			}
			if got := s.NeedsInstall(); got != tt.want {
				t.Fatalf("NeedsInstall = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMissingMPV(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	s, err := installed(t.Context())
	if err != nil || s.Path != "" {
		t.Fatalf("missing mpv: %+v, %v", s, err)
	}
}

func TestLatestReleaseCacheAndOffline(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("User-Agent") != "plexctl" {
			t.Error("missing release client identification")
		}
		_, _ = w.Write([]byte(`{"tag_name":"v0.42.0","prerelease":false}`))
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	for range 2 {
		v, err := latest(t.Context(), srv.Client(), srv.URL, dir, false)
		if err != nil || v != "0.42.0" {
			t.Fatalf("latest: %q, %v", v, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("daily cache made %d requests", calls.Load())
	}
	if _, err := latest(t.Context(), srv.Client(), srv.URL, dir, true); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("explicit refresh did not bypass cache")
	}
	srv.Close()
	v, err := latest(t.Context(), srv.Client(), srv.URL, dir, true)
	if err == nil || v != "0.42.0" {
		t.Fatalf("offline fallback: %q, %v", v, err)
	}
}

func TestLatestRejectsInvalidRelease(t *testing.T) {
	for _, body := range []string{
		`{"tag_name":"git-release","prerelease":true}`,
		`{"tag_name":"v0.42.0-rc1","prerelease":true}`,
		`{"tag_name":"v0.42.0","draft":true}`,
		`{"tag_name":"v0.42.0; touch /tmp/unsafe"}`,
		`{"tag_name":"v999999999999999999999999.0.0"}`,
		`<html>unavailable</html>`,
	} {
		t.Run(body, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer srv.Close()
			if v, err := latest(t.Context(), srv.Client(), srv.URL, t.TempDir(), true); err == nil || v != "" {
				t.Fatalf("accepted invalid release: %q, %v", v, err)
			}
		})
	}
}

func TestLatestExpiredCacheAndRateLimit(t *testing.T) {
	dir := t.TempDir()
	data, err := json.Marshal(releaseCache{Version: "0.41.0", Checked: time.Now().Add(-48 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mpv-release.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer srv.Close()
	v, err := latest(t.Context(), srv.Client(), srv.URL, dir, false)
	if v != "0.41.0" || err == nil {
		t.Fatalf("rate-limited stale cache: %q, %v", v, err)
	}
}

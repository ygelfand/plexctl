package plex

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/ygelfand/plexctl/internal/config"
)

func TestLoggingTransportOmitsCredentialsAndPreservesBodies(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	level := new(slog.LevelVar)
	level.Set(config.LevelTrace)
	cfg := &config.Config{
		Verbosity: 3, LogLevel: level,
		Logger: slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: level})),
	}
	const requestBody = `{"pin":"private-pin"}`
	const responseBody = `{"authToken":"response-secret"}`
	req, err := http.NewRequest(http.MethodPost,
		"https://user:private-password@plex.invalid/switch?X-Plex-Token=query-secret", strings.NewReader(requestBody))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Plex-Token", "header-secret")
	transport := &loggingTransport{cfg: cfg, base: resourceTransport(func(r *http.Request) (*http.Response, error) {
		data, err := io.ReadAll(r.Body)
		if err != nil || string(data) != requestBody {
			t.Fatalf("logging consumed or changed the request body: %v", err)
		}
		return &http.Response{
			StatusCode: 200, Header: http.Header{"X-Plex-Token": {"response-header-secret"}},
			Body: io.NopCloser(strings.NewReader(responseBody)),
		}, nil
	})}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil || string(data) != responseBody {
		t.Fatalf("logging consumed or changed the response body: %v", err)
	}
	for _, secret := range []string{"private-pin", "response-secret", "private-password", "query-secret", "header-secret", "response-header-secret"} {
		if strings.Contains(output.String(), secret) {
			t.Error("verbose SDK logging exposed a credential")
		}
	}
	if !strings.Contains(output.String(), "SDK Request") || !strings.Contains(output.String(), "plex.invalid/switch") {
		t.Fatal("request diagnostics were lost")
	}
}

func TestHasServerRequiresConfiguredEndpoint(t *testing.T) {
	cfg := config.Get()
	old := *cfg
	t.Cleanup(func() { *cfg = old })
	cfg.Servers = map[string]config.Server{
		"configured": {URL: "https://plex.invalid"},
		"empty":      {},
	}
	client := &Client{}
	for _, test := range []struct {
		name, id string
		want     bool
	}{
		{"no default", "", false},
		{"unknown default", "deleted-server", false},
		{"missing endpoint", "empty", false},
		{"configured", "configured", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg.DefaultServer = test.id
			if got := client.HasServer(); got != test.want {
				t.Fatalf("HasServer() = %v, want %v", got, test.want)
			}
		})
	}
}

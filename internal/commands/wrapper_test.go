package commands

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
)

type commandTransport func(*http.Request) (*http.Response, error)

func (f commandTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCommandClientsSeparateAccountAndServerTokens(t *testing.T) {
	t.Setenv("PLEXCTL_TOKEN", "")
	cfg := config.Get()
	old := *cfg
	t.Cleanup(func() { *cfg = old })
	cfg.Token = "account-token"
	cfg.HomeUser = config.HomeUser{}
	cfg.DefaultServer = "shared"
	cfg.Servers = map[string]config.Server{"shared": {
		URL: "https://friend.invalid", Shared: true,
		AccessToken: "resource-token", TokenSource: cfg.TokenSource(),
	}}
	base := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = base })
	requests := 0
	http.DefaultTransport = commandTransport(func(req *http.Request) (*http.Response, error) {
		requests++
		body := `{"MediaContainer":{"Directory":[]}}`
		want := "resource-token"
		if req.URL.Host == "plex.tv" {
			want = "account-token"
			body = `{"users":[]}`
		} else if req.URL.Host != "friend.invalid" {
			t.Errorf("unexpected origin %q", req.URL.Host)
		}
		if got := req.Header.Get("X-Plex-Token"); got != want {
			t.Errorf("wrong credential for %s", req.URL.Host)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}},
			Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})
	account := RunWithClient(func(ctx context.Context, client *plex.Client, _ *cobra.Command, _ []string, _ *PlexCtlOptions) error {
		_, err := client.SDK.HomeUsers.GetHomeUsers(ctx)
		return err
	})
	server := RunWithServer(func(ctx context.Context, client *plex.Client, _ *cobra.Command, _ []string, _ *PlexCtlOptions) error {
		_, err := client.SDK.Library.GetSections(ctx)
		return err
	})
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	if err := account(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if err := server(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("got %d requests, want 2", requests)
	}
}

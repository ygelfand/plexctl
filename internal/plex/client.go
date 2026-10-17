package plex

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/LukeHagar/plexgo"
	"github.com/ygelfand/plexctl/internal/config"
)

type Client struct {
	SDK *plexgo.PlexAPI
}

type loggingTransport struct {
	base http.RoundTripper
	cfg  *config.Config
}

func (t *loggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Headers, bodies, and query strings can contain account tokens, server
	// resource tokens, or a Home user's PIN. Log only request metadata.
	t.cfg.Logger.Debug("SDK Request", "method", req.Method, "url", safeURL(req.URL))

	start := time.Now()
	res, err := t.base.RoundTrip(req)
	duration := time.Since(start)

	if err != nil {
		t.cfg.Logger.Error("SDK Request Failed", "error_type", fmt.Sprintf("%T", err), "duration", duration)
		return nil, err
	}

	t.cfg.Logger.Debug("SDK Response", "status", res.StatusCode, "duration", duration)

	return res, nil
}

func NewClient() (*Client, error) {
	return NewClientWithToken(config.Get().ServerToken())
}

// NewAccountClient uses the active Plex account or Home profile for plex.tv.
func NewAccountClient() (*Client, error) {
	return NewAccountClientWithToken(config.Get().AccountToken())
}

// NewAccountClientWithToken keeps a captured account credential independent of
// the configured media server, including during a Home profile switch.
func NewAccountClientWithToken(token string) (*Client, error) {
	return newClient(token, "")
}

// NewHomeHomeUserClient specifically uses the general user token.
func NewHomeHomeUserClient() (*Client, error) {
	return NewAccountClient()
}

func NewClientWithToken(token string) (*Client, error) {
	cfg := config.Get()
	_, serverCfg, hasServer := cfg.GetActiveServer()
	serverURL := ""
	if hasServer {
		serverURL = serverCfg.URL
	}
	return newClient(token, serverURL)
}

func NewClientForContext(ctx context.Context) (*Client, error) {
	server := config.ServerFromContext(ctx)
	return newClient(server.Token, server.URL)
}

func newClient(token, serverURL string) (*Client, error) {
	cfg := config.Get()

	if token == "" {
		return nil, fmt.Errorf("plex token not found. please login with 'plexctl login' or set PLEXCTL_TOKEN")
	}
	httpClient := &http.Client{
		Transport: &loggingTransport{
			base: http.DefaultTransport,
			cfg:  cfg,
		},
		Timeout:       60 * time.Second,
		CheckRedirect: sameOriginRedirect,
	}

	opts := []plexgo.SDKOption{
		plexgo.WithSecurity(token),
		plexgo.WithClient(httpClient),
		plexgo.WithClientIdentifier(config.ClientIdentifier()),
		plexgo.WithProduct("plexctl"),
		plexgo.WithDevice(runtime.GOOS),
		plexgo.WithDeviceName(config.GetHostname()),
		plexgo.WithPlatform(runtime.GOOS),
		plexgo.WithPlatformVersion(runtime.GOARCH),
		plexgo.WithVersion(config.Version),
	}

	if serverURL != "" {
		opts = append(opts, plexgo.WithServerURL(serverURL))
	}

	return &Client{SDK: plexgo.New(opts...)}, nil
}

// HasServer returns true if a default server is configured
func (c *Client) HasServer() bool {
	_, server, ok := config.Get().GetActiveServer()
	return ok && server.URL != ""
}

package plex

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"os"
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
	t.cfg.Logger.Debug("SDK Request", "method", req.Method, "url", req.URL.String())

	if t.cfg.Enabled(config.LevelTrace) {
		dump, err := httputil.DumpRequestOut(req, t.cfg.Verbosity >= 3)
		if err == nil {
			t.cfg.Logger.Log(context.Background(), config.LevelTrace, "SDK Request Dump", "dump", string(dump))
		}
	}

	start := time.Now()
	res, err := t.base.RoundTrip(req)
	duration := time.Since(start)

	if err != nil {
		t.cfg.Logger.Error("SDK Request Failed", "error", err, "duration", duration)
		return nil, err
	}

	t.cfg.Logger.Debug("SDK Response", "status", res.Status, "duration", duration)

	if t.cfg.Enabled(config.LevelTrace) {
		dump, err := httputil.DumpResponse(res, t.verbosityBody())
		if err == nil {
			t.cfg.Logger.Log(context.Background(), config.LevelTrace, "SDK Response Dump", "dump", string(dump))
		}
	}

	return res, nil
}

func (t *loggingTransport) verbosityBody() bool {
	return t.cfg.Verbosity >= 3
}

func NewClient() (*Client, error) {
	cfg := config.Get()

	// 1. Check for Token (Global)
	token := os.Getenv("PLEXCTL_TOKEN")
	source := "env"
	if token == "" {
		token = cfg.Token
		source = "main account"
		// If a home user access token is set, it overrides the main account token
		if cfg.HomeUser.AccessToken != "" {
			token = cfg.HomeUser.AccessToken
			source = "home user access"
		}
	}

	slog.Debug("NewClient: Initializing with token", "source", source)
	return NewClientWithToken(token)
}

// NewHomeUserClient specifically uses the V2 AuthToken (general user token)
func NewHomeHomeUserClient() (*Client, error) {
	cfg := config.Get()
	token := cfg.Token
	if cfg.HomeUser.AuthToken != "" {
		token = cfg.HomeUser.AuthToken
	}
	return NewClientWithToken(token)
}

func NewClientWithToken(token string) (*Client, error) {
	cfg := config.Get()
	// 2. Resolve Server (if configured)
	_, serverCfg, hasServer := cfg.GetActiveServer()

	if token == "" {
		return nil, fmt.Errorf("plex token not found. please login with 'plexctl login' or set PLEXCTL_TOKEN")
	}
	httpClient := &http.Client{
		Transport: &loggingTransport{
			base: http.DefaultTransport,
			cfg:  cfg,
		},
		Timeout: 60 * time.Second,
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

	if hasServer && serverCfg.URL != "" {
		opts = append(opts, plexgo.WithServerURL(serverCfg.URL))
	}

	return &Client{SDK: plexgo.New(opts...)}, nil
}

// HasServer returns true if a default server is configured
func (c *Client) HasServer() bool {
	return config.Get().DefaultServer != ""
}

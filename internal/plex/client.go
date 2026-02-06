package plex

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
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
	token := cfg.Token

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
	} else {
		// If no server is configured, we can still return a client, but it effectively points nowhere valid for PMS commands.
		// It IS valid for plex.tv commands (like GetServerResources) which are global.
	}

	return &Client{SDK: plexgo.New(opts...)}, nil
}

// HasServer returns true if a default server is configured
func (c *Client) HasServer() bool {
	return config.Get().DefaultServer != ""
}

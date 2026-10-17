package plex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/ygelfand/plexctl/internal/config"
)

// DiscoverServers includes servers shared with the active Plex account/profile.
func DiscoverServers(ctx context.Context) ([]components.PlexDevice, error) {
	client, err := NewAccountClientWithToken(config.ServerFromContext(ctx).AccountToken)
	if err != nil {
		return nil, err
	}
	res, err := client.SDK.Plex.GetServerResources(ctx, operations.GetServerResourcesRequest{
		IncludeHTTPS: operations.IncludeHTTPSTrue.ToPointer(),
		IncludeIPv6:  operations.IncludeIPv6True.ToPointer(),
		IncludeRelay: operations.IncludeRelayTrue.ToPointer(),
	})
	if err != nil {
		return nil, fmt.Errorf("discover Plex servers: %w", err)
	}
	return mediaServers(res.PlexDevices), nil
}

func mediaServers(devices []components.PlexDevice) []components.PlexDevice {
	var servers []components.PlexDevice
	for _, device := range devices {
		if slices.Contains(strings.Split(device.Provides, ","), "server") {
			servers = append(servers, device)
		}
	}
	slices.SortStableFunc(servers, func(a, b components.PlexDevice) int {
		if a.Owned != b.Owned {
			if a.Owned {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return servers
}

// ConnectServer tries advertised direct connections before Plex Relay. Only the
// resource's token is sent; an account token must never be sent to a friend's PMS.
func ConnectServer(ctx context.Context, device components.PlexDevice) (config.ServerTarget, error) {
	if device.ClientIdentifier == "" {
		return config.ServerTarget{}, fmt.Errorf("missing Plex server identity")
	}
	if device.AccessToken == "" {
		return config.ServerTarget{}, fmt.Errorf("no access token for %s; refresh your Plex login or library invitation", device.Name)
	}
	connections := slices.Clone(device.Connections)
	slices.SortStableFunc(connections, func(a, b components.Connections) int {
		return connectionRank(a, device.Owned) - connectionRank(b, device.Owned)
	})
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for _, conn := range connections {
		u, err := url.Parse(conn.URI)
		if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
			(u.Scheme != "https" && u.Scheme != "http") || (device.HTTPSRequired && u.Scheme != "https") {
			continue
		}
		target := config.ServerTarget{ID: device.ClientIdentifier, Name: device.Name, URL: strings.TrimRight(conn.URI, "/"), Token: device.AccessToken, Shared: !device.Owned}
		if probeServer(ctx, client, target) == nil {
			return target, nil
		}
		if ctx.Err() != nil {
			break
		}
	}
	return config.ServerTarget{}, fmt.Errorf("cannot reach %s or access its libraries; check that the server is online and sharing is still enabled", device.Name)
}

func connectionRank(conn components.Connections, owned bool) int {
	rank := 0
	if conn.Relay {
		rank += 8
	}
	if conn.Local != owned {
		rank += 4
	}
	if !strings.HasPrefix(conn.URI, "https://") {
		rank++
	}
	return rank
}

func probeServer(ctx context.Context, client *http.Client, target config.ServerTarget) error {
	// Checking identity avoids mistaking a friend's private LAN address for a
	// different Plex server at the same address on the current user's network.
	for _, path := range []string{"/identity", "/library/sections"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.URL+path, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")
		// The identity endpoint is public. First verify that an advertised
		// private address belongs to this server, then send its access token.
		if path != "/identity" {
			req.Header.Set("X-Plex-Token", target.Token)
		}
		req.Header.Set("X-Plex-Client-Identifier", config.ClientIdentifier())
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("connection failed")
		}
		var body struct {
			MediaContainer *struct {
				MachineIdentifier string `json:"machineIdentifier"`
			} `json:"MediaContainer"`
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK || err != nil || body.MediaContainer == nil {
			return fmt.Errorf("server did not allow library access")
		}
		if path == "/identity" && body.MediaContainer.MachineIdentifier != target.ID {
			return fmt.Errorf("server identity mismatch")
		}
	}
	return nil
}

func ServerDescription(device components.PlexDevice) string {
	if device.Owned {
		return "Owned"
	}
	if device.SourceTitle != nil && *device.SourceTitle != "" {
		return "Shared by " + *device.SourceTitle
	}
	return "Shared with you"
}

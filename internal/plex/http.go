package plex

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ygelfand/plexctl/internal/config"
)

// MediaURL builds a stream URL without allowing server metadata to change the
// destination or supply its own credentials.
func MediaURL(server config.ServerTarget, path string) (string, error) {
	u, err := mediaURL(server, path)
	if err != nil {
		return "", err
	}
	query := u.Query()
	query.Set("X-Plex-Token", server.Token)
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func mediaURL(server config.ServerTarget, path string) (*url.URL, error) {
	base, err := url.Parse(server.URL)
	if err != nil || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" ||
		(base.Scheme != "https" && base.Scheme != "http") {
		return nil, fmt.Errorf("invalid Plex server URL")
	}
	ref, err := url.Parse(path)
	if err != nil || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") ||
		ref.IsAbs() || ref.Host != "" || ref.User != nil || ref.Fragment != "" {
		return nil, fmt.Errorf("invalid Plex media path")
	}
	// Assign path fields rather than concatenating complete URLs: a path such
	// as @another-host/ must never become URL userinfo and change the host.
	base.RawPath = strings.TrimRight(base.EscapedPath(), "/") + ref.EscapedPath()
	base.Path = strings.TrimRight(base.Path, "/") + ref.Path
	query, err := url.ParseQuery(ref.RawQuery)
	if err != nil {
		return nil, fmt.Errorf("invalid Plex media query")
	}
	for key := range query {
		if strings.EqualFold(key, "X-Plex-Token") {
			delete(query, key)
		}
	}
	base.RawQuery = query.Encode()
	return base, nil
}

func safeURL(u *url.URL) string {
	return u.Scheme + "://" + u.Host + u.EscapedPath()
}

func sameOriginRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("stopped after 10 redirects")
	}
	if len(via) == 0 {
		return nil
	}
	origin := via[0].URL
	if req.URL.User != nil || !strings.EqualFold(req.URL.Scheme, origin.Scheme) ||
		!strings.EqualFold(req.URL.Host, origin.Host) {
		return http.ErrUseLastResponse
	}
	return nil
}

func mediaHTTPClient() *http.Client {
	return &http.Client{Timeout: 60 * time.Second, CheckRedirect: sameOriginRedirect}
}

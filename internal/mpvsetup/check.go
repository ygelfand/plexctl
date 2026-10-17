// Package mpvsetup checks the playback dependency and offers approved updates.
package mpvsetup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const releaseURL = "https://api.github.com/repos/mpv-player/mpv/releases/latest"

type Status struct {
	Path      string
	Installed string
	Latest    string
	CheckErr  error
}

func (s Status) NeedsInstall() bool {
	return s.Path == "" || s.Latest != "" && older(s.Installed, s.Latest)
}

var versionPattern = regexp.MustCompile(`^v?([0-9]+)\.([0-9]+)\.([0-9]+)([-+].*)?$`)

func version(value string) ([3]int, bool) {
	var parts [3]int
	match := versionPattern.FindStringSubmatch(value)
	if match == nil {
		return parts, false
	}
	for i := range parts {
		n, err := strconv.Atoi(match[i+1])
		if err != nil {
			return parts, false
		}
		parts[i] = n
	}
	return parts, true
}

func older(installed, latest string) bool {
	a, okA := version(installed)
	b, okB := version(latest)
	if !okA || !okB {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	// git snapshots after a stable release must never be downgraded to it.
	return strings.Contains(installed, "-rc")
}

func installed(ctx context.Context) (Status, error) {
	path, err := exec.LookPath("mpv")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return Status{}, nil
		}
		return Status{}, fmt.Errorf("find mpv: %w", err)
	}
	return inspect(ctx, path)
}

func inspect(ctx context.Context, path string) (Status, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--no-config", "--version")
	// Wrappers can leave descendants holding stdout after the probe exits.
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if err != nil {
		return Status{}, fmt.Errorf("check mpv at %s: %w", path, err)
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 || fields[0] != "mpv" {
		return Status{}, fmt.Errorf("unrecognized mpv version at %s", path)
	}
	if _, ok := version(fields[1]); !ok {
		return Status{}, fmt.Errorf("unrecognized mpv version at %s", path)
	}
	return Status{Path: path, Installed: strings.TrimPrefix(fields[1], "v")}, nil
}

func stableVersion(v string) bool {
	_, ok := version(v)
	return ok && !strings.ContainsAny(v, "-+")
}

type releaseCache struct {
	Version string    `json:"version"`
	Checked time.Time `json:"checked"`
}

// Check uses a daily cache and a short deadline. An unavailable release service
// must not prevent an installed player from working offline.
func Check(ctx context.Context, cacheDir string, refresh bool) (Status, error) {
	s, err := installed(ctx)
	if err != nil {
		return s, err
	}
	s.Latest, s.CheckErr = latest(ctx, http.DefaultClient, releaseURL, cacheDir, refresh)
	return s, nil
}

func latest(ctx context.Context, client *http.Client, endpoint, cacheDir string, refresh bool) (string, error) {
	path := filepath.Join(cacheDir, "mpv-release.json")
	var cached releaseCache
	if cacheDir != "" {
		data, _ := os.ReadFile(path)
		_ = json.Unmarshal(data, &cached)
		if !stableVersion(cached.Version) {
			cached = releaseCache{}
		}
	}
	if age := time.Since(cached.Checked); !refresh && cached.Version != "" && age >= 0 && age < 24*time.Hour {
		return cached.Version, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return cached.Version, fmt.Errorf("check latest mpv release: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "plexctl")
	res, err := client.Do(req)
	if err != nil {
		return cached.Version, fmt.Errorf("latest mpv release is unavailable")
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return cached.Version, fmt.Errorf("latest mpv release check returned HTTP %d", res.StatusCode)
	}
	var release struct {
		Tag        string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
		Draft      bool   `json:"draft"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&release); err != nil {
		return cached.Version, fmt.Errorf("invalid mpv release response")
	}
	v := strings.TrimPrefix(release.Tag, "v")
	if !stableVersion(v) || release.Prerelease || release.Draft {
		return cached.Version, fmt.Errorf("mpv release response did not identify a stable version")
	}
	if cacheDir != "" {
		data, _ := json.Marshal(releaseCache{Version: v, Checked: time.Now()})
		if os.MkdirAll(cacheDir, 0o755) == nil {
			_ = os.WriteFile(path, data, 0o600)
		}
	}
	return v, nil
}

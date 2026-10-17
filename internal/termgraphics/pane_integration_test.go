//go:build integration && !windows

package termgraphics

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPaneZoomPreservesLaterTmuxLayout(t *testing.T) {
	tmux, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux is not installed")
	}
	for _, scenario := range []string{"peer zoomed", "pane moved", "fullscreen toggled twice"} {
		t.Run(scenario, func(t *testing.T) {
			// Keep the Unix socket path short and use an isolated server.
			dir, err := os.MkdirTemp("", "plexctl-tmux-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(dir) })
			socket := filepath.Join(dir, "socket")
			run := func(args ...string) ([]byte, error) {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				return exec.CommandContext(ctx, tmux, append([]string{"-S", socket, "-f", "/dev/null"}, args...)...).CombinedOutput()
			}
			must := func(args ...string) string {
				t.Helper()
				out, err := run(args...)
				if err != nil {
					t.Fatalf("tmux %v: %v\n%s", args, err, out)
				}
				return strings.TrimSpace(string(out))
			}
			t.Cleanup(func() { _, _ = run("kill-server") })
			pane := must("new-session", "-d", "-s", "review", "-x", "100", "-y", "30", "-P", "-F", "#{pane_id}")
			peer := must("split-window", "-d", "-h", "-t", pane, "-P", "-F", "#{pane_id}")
			p, err := newPaneZoom(pane, run)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := p.Toggle(); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "peer zoomed":
				must("resize-pane", "-Z", "-t", pane)
				must("resize-pane", "-Z", "-t", peer)
			case "pane moved":
				must("break-pane", "-d", "-s", pane, "-t", "review:")
				must("split-window", "-d", "-h", "-t", pane)
				must("resize-pane", "-Z", "-t", pane)
			case "fullscreen toggled twice":
				if _, err := p.Toggle(); err != nil {
					t.Fatal(err)
				}
				must("resize-pane", "-Z", "-t", pane)
			}
			before := must("list-panes", "-a", "-F", "#{window_id} #{pane_id} #{pane_active} #{window_zoomed_flag}")
			if err := p.Restore(); err != nil {
				t.Fatal(err)
			}
			after := must("list-panes", "-a", "-F", "#{window_id} #{pane_id} #{pane_active} #{window_zoomed_flag}")
			if after != before {
				t.Fatalf("exiting playback changed the user's later layout:\nbefore:\n%s\nafter:\n%s", before, after)
			}
		})
	}
}

package termgraphics

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// PaneZoom tracks playback's explicit fullscreen changes. Creating it only
// reads the current layout; playback starts at the pane's existing size.
// Its methods belong to the playback event loop, not concurrent callers.
type PaneZoom struct {
	pane    string
	run     func(...string) ([]byte, error)
	initial paneZoomState
	last    paneZoomState
	changed bool
}

type paneZoomState struct {
	window         string
	zoomed, active bool
}

// NewPaneZoom returns nil outside an actual tmux pane.
func NewPaneZoom() (*PaneZoom, error) {
	if !InTmux() {
		return nil, nil
	}
	return newPaneZoom(os.Getenv("TMUX_PANE"), func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "tmux", args...)
		cmd.WaitDelay = time.Second
		return cmd.Output()
	})
}

func newPaneZoom(pane string, run func(...string) ([]byte, error)) (*PaneZoom, error) {
	p := &PaneZoom{pane: pane, run: run}
	initial, err := p.state()
	if err != nil {
		return nil, err
	}
	p.initial, p.last = initial, initial
	return p, nil
}

func (p *PaneZoom) Zoomed() (bool, error) {
	state, err := p.state()
	return state.zoomed && state.active, err
}

func (p *PaneZoom) state() (paneZoomState, error) {
	value, err := p.run("display-message", "-p", "-t", p.pane, "#{window_id} #{window_zoomed_flag} #{pane_active}")
	if err != nil {
		return paneZoomState{}, fmt.Errorf("inspect tmux pane zoom: %w", err)
	}
	fields := strings.Fields(string(value))
	if len(fields) != 3 || !strings.HasPrefix(fields[0], "@") ||
		(fields[1] != "0" && fields[1] != "1") || (fields[2] != "0" && fields[2] != "1") {
		return paneZoomState{}, fmt.Errorf("tmux did not report the playback pane's zoom state")
	}
	return paneZoomState{window: fields[0], zoomed: fields[1] == "1", active: fields[2] == "1"}, nil
}

// Origin is the pane's position in the attached terminal, including a status
// bar above the window. tmux does not position the cursor for raw passthrough.
func (p *PaneZoom) Origin() (int, int, error) {
	value, err := p.run("display-message", "-p", "-t", p.pane, "#{pane_left} #{pane_top} #{status-position} #{status}")
	if err != nil {
		return 0, 0, fmt.Errorf("inspect tmux pane position: %w", err)
	}
	fields := strings.Fields(string(value))
	if len(fields) != 4 {
		return 0, 0, fmt.Errorf("tmux did not report the playback pane's position")
	}
	x, xerr := strconv.Atoi(fields[0])
	y, yerr := strconv.Atoi(fields[1])
	if xerr != nil || yerr != nil || x < 0 || y < 0 {
		return 0, 0, fmt.Errorf("tmux reported an invalid playback pane position")
	}
	if fields[2] == "top" {
		lines := 0
		switch fields[3] {
		case "on":
			lines = 1
		case "off":
		default:
			lines, err = strconv.Atoi(fields[3])
			if err != nil || lines < 0 || lines > 5 {
				return 0, 0, fmt.Errorf("tmux reported an invalid status bar height")
			}
		}
		y += lines
	}
	return x, y, nil
}

func (p *PaneZoom) Toggle() (bool, error) {
	before, err := p.state()
	if err != nil {
		return false, err
	}
	// A later explicit toggle starts from the user's current layout, including
	// changes made outside playback or moving the pane to another window.
	if !p.changed || before != p.last {
		p.initial = before
	}
	if _, err := p.run("resize-pane", "-Z", "-t", p.pane); err != nil {
		return false, fmt.Errorf("toggle terminal playback fullscreen: %w", err)
	}
	// tmux leaves a one-pane window unzoomed. Read the actual result instead
	// of assuming every successful command changed the state.
	after, err := p.state()
	if err != nil {
		return false, err
	}
	p.last = after
	p.changed = after.zoomed != p.initial.zoomed
	return after.zoomed && after.active, nil
}

// Restore undoes playback's outstanding zoom change only while the same pane
// and window still have the state left by its last fullscreen toggle.
func (p *PaneZoom) Restore() error {
	if !p.changed {
		return nil
	}
	current, err := p.state()
	if err != nil {
		return err
	}
	if current == p.last && current.zoomed != p.initial.zoomed {
		if _, err := p.Toggle(); err != nil {
			return err
		}
	}
	p.changed = false
	return nil
}

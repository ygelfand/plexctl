package termgraphics

import (
	"errors"
	"fmt"
	"slices"
	"testing"
)

func TestPaneZoomKeepsInitialLayout(t *testing.T) {
	for _, tc := range []struct {
		name    string
		initial bool
		toggles int
	}{
		{"split unchanged", false, 0}, {"already zoomed unchanged", true, 0},
		{"restore split after fullscreen", false, 1}, {"restore original zoom", true, 1},
		{"fullscreen toggled twice", false, 2}, {"original zoom toggled twice", true, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			zoomed, mutations := tc.initial, 0
			run := func(args ...string) ([]byte, error) {
				switch {
				case slices.Equal(args, []string{"display-message", "-p", "-t", "%42", "#{window_id} #{window_zoomed_flag} #{pane_active}"}):
					if zoomed {
						return []byte("@1 1 1\n"), nil
					}
					return []byte("@1 0 1\n"), nil
				case slices.Equal(args, []string{"resize-pane", "-Z", "-t", "%42"}):
					mutations++
					zoomed = !zoomed
					return nil, nil
				default:
					return nil, fmt.Errorf("unexpected tmux command: %q", args)
				}
			}
			p, err := newPaneZoom("%42", run)
			if err != nil {
				t.Fatal(err)
			}
			if mutations != 0 || zoomed != tc.initial {
				t.Fatal("starting playback changed the pane layout")
			}
			for range tc.toggles {
				before := zoomed
				got, err := p.Toggle()
				if err != nil || got == before || got != zoomed {
					t.Fatalf("toggle: zoomed=%v, err=%v", got, err)
				}
			}
			if err := p.Restore(); err != nil {
				t.Fatal(err)
			}
			if zoomed != tc.initial {
				t.Fatal("playback did not restore the original zoom")
			}
			before := mutations
			if err := p.Restore(); err != nil || mutations != before {
				t.Fatal("restoration was not idempotent", err)
			}
			if tc.toggles == 0 && mutations != 0 {
				t.Fatal("playback zoomed without user input")
			}
		})
	}
}

func TestPaneZoomLeavesExternalChangesAlone(t *testing.T) {
	state := "@1 0 1"
	p, err := newPaneZoom("%42", func(args ...string) ([]byte, error) {
		if args[0] != "display-message" {
			t.Fatal("restoration changed an externally zoomed pane")
		}
		return []byte(state), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	state = "@1 1 1"
	if err := p.Restore(); err != nil {
		t.Fatal(err)
	}
}

func TestPaneOrigin(t *testing.T) {
	for _, tc := range []struct {
		name, reply string
		x, y        int
		wantErr     bool
	}{
		{"right split", "74 0 bottom on", 74, 0, false},
		{"lower split", "0 25 bottom on", 0, 25, false},
		{"status above", "12 8 top on", 12, 9, false},
		{"two status rows", "12 8 top 2", 12, 10, false},
		{"no status", "12 8 top off", 12, 8, false},
		{"missing pane", "", 0, 0, true},
		{"invalid coordinates", "bad 8 top on", 0, 0, true},
		{"negative coordinates", "-1 0 bottom on", 0, 0, true},
		{"invalid status", "0 0 top invalid", 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &PaneZoom{pane: "%42", run: func(args ...string) ([]byte, error) {
				if !slices.Equal(args, []string{"display-message", "-p", "-t", "%42", "#{pane_left} #{pane_top} #{status-position} #{status}"}) {
					t.Fatalf("unexpected position query: %q", args)
				}
				return []byte(tc.reply), nil
			}}
			x, y, err := p.Origin()
			if (err != nil) != tc.wantErr || x != tc.x || y != tc.y {
				t.Fatalf("origin = %d,%d, %v; want %d,%d, error=%v", x, y, err, tc.x, tc.y, tc.wantErr)
			}
		})
	}
}

func TestPaneZoomFailureAndSinglePane(t *testing.T) {
	failed := errors.New("tmux failed")
	if _, err := newPaneZoom("%42", func(...string) ([]byte, error) { return nil, failed }); !errors.Is(err, failed) {
		t.Fatalf("inspect error lost: %v", err)
	}
	if _, err := newPaneZoom("%42", func(...string) ([]byte, error) { return []byte(""), nil }); err == nil {
		t.Fatal("missing pane state was accepted")
	}
	p, err := newPaneZoom("%42", func(args ...string) ([]byte, error) {
		if args[0] == "resize-pane" {
			return nil, failed
		}
		return []byte("@1 0 1"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Toggle(); !errors.Is(err, failed) {
		t.Fatalf("toggle error lost: %v", err)
	}
	if err := p.Restore(); err != nil {
		t.Fatal("failed toggle should not need restoration", err)
	}
	p, err = newPaneZoom("%42", func(...string) ([]byte, error) { return []byte("@1 0 1"), nil })
	if err != nil {
		t.Fatal(err)
	}
	if zoomed, err := p.Toggle(); err != nil || zoomed {
		t.Fatalf("single pane was incorrectly marked fullscreen: %v, %v", zoomed, err)
	}
}

func TestPaneZoomOutsideTmux(t *testing.T) {
	t.Setenv("TMUX", "")
	p, err := NewPaneZoom()
	if err != nil || p != nil {
		t.Fatalf("unexpected tmux controller outside tmux: %v, %v", p, err)
	}
}

package config

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ygelfand/plexctl/internal/plex"
)

func TestLibraryClickBesideEmptyColumn(t *testing.T) {
	for _, width := range []int{50, 83, 120} {
		m := NewConfigOverlayModel([]plex.LibraryInfo{{ID: "test", Title: "Mouse QA"}})
		m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		view := m.View()
		if lipgloss.Width(view) > width {
			t.Fatalf("dialog width %d exceeds terminal width %d", lipgloss.Width(view), width)
		}
		clicked := false
		for y, line := range strings.Split(ansi.Strip(view), "\n") {
			if before, _, ok := strings.Cut(line, "Mouse QA"); ok {
				m.Update(tea.MouseMsg{X: lipgloss.Width(before) + 1, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
				clicked = true
				break
			}
		}
		if !clicked || !m.isPicking || m.focusSide != 1 {
			t.Fatalf("width %d: active-column click did not open its icon picker (side=%d)", width, m.focusSide)
		}
	}
}

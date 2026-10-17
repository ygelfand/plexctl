package poster

import (
	"fmt"
	"testing"

	"github.com/LukeHagar/plexgo/models/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ygelfand/plexctl/internal/ui"
)

func TestPosterGridMouseAfterScrollingAndResize(t *testing.T) {
	ui.GetLayout().Update(90, 25, false)
	m := NewPosterGrid(nil, ui.PlexctlTheme)
	for i := range 30 {
		m.Items = append(m.Items, &PosterItem{Metadata: components.Metadata{Title: fmt.Sprintf("Movie %d", i)}})
	}
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 25})
	m.View()
	for range 6 {
		m.Update(tea.MouseMsg{X: 5, Y: 5, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	}
	_, cmd := m.Update(tea.MouseMsg{X: ui.PosterStepWidth + 2, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if cmd == nil {
		t.Fatal("visible poster did not open")
	}
	selected := cmd().(ItemSelectedMsg)
	if selected.Metadata.Title != "Movie 4" {
		t.Fatalf("scrolled click selected %s", selected.Metadata.Title)
	}
	_, cmd = m.Update(tea.MouseMsg{X: ui.PosterStepWidth - 1, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if cmd != nil {
		t.Fatal("poster margin activated a movie")
	}
	ui.GetLayout().Update(50, 25, false)
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 25})
	if m.Cursor != 4 {
		t.Fatalf("resize changed selection to %d", m.Cursor)
	}
}

func TestPosterHeightDoesNotChangeWithSelection(t *testing.T) {
	item := &PosterItem{Metadata: components.Metadata{Title: "A long movie title that wraps"}}
	for _, selected := range []bool{false, true} {
		view := RenderPosterItem(item, selected, true, ui.PlexctlTheme)
		if height := lipgloss.Height(view); height != ui.PosterTotalHeight {
			t.Fatalf("selected %v: poster height %d, want %d", selected, height, ui.PosterTotalHeight)
		}
	}
}

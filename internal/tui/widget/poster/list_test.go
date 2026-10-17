package poster

import (
	"testing"

	"github.com/LukeHagar/plexgo/models/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ygelfand/plexctl/internal/ui"
)

func TestPosterListResizeKeepsSelectionVisible(t *testing.T) {
	ui.GetLayout().Update(ui.SidebarTotalWidth+ui.MainOverhead+6*ui.PosterStepWidth, 50, false)
	m := NewPosterList(0, "Movies", make([]components.Metadata, 12), ui.PlexctlTheme)
	m.Cursor = 5
	m.Sync()
	ui.GetLayout().Update(ui.SidebarTotalWidth+ui.MainOverhead+3*ui.PosterStepWidth, 50, false)
	m.Update(tea.WindowSizeMsg{})
	if m.Cursor != 5 || m.Cursor < m.Offset || m.Cursor >= m.Offset+3 {
		t.Fatalf("resize left selection %d outside the row beginning at %d", m.Cursor, m.Offset)
	}
}

func TestPosterSelectionRetainsItemWhenListChangesBeforeDispatch(t *testing.T) {
	items := []components.Metadata{{Title: "Selected"}, {Title: "Other"}}
	list := NewPosterList(0, "Movies", items, ui.PlexctlTheme)
	list.Active = true
	grid := NewPosterGrid(items, ui.PlexctlTheme)
	for _, model := range []tea.Model{list, grid} {
		_, cmd := model.Update(ui.KeyPress("enter"))
		if cmd == nil {
			t.Fatal("selection did not produce a command")
		}
		switch model := model.(type) {
		case *PosterList:
			model.SetItems(nil)
		case *PosterGrid:
			model.SetItems(nil)
		}
		if selection := cmd().(ItemSelectedMsg); selection.Metadata.Title != "Selected" {
			t.Fatal("queued selection changed after the list was refreshed")
		}
	}
}

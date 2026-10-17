package view

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/ui"
)

func TestSearchStatusNavigationWhileIndexing(t *testing.T) {
	for _, test := range []struct {
		name     string
		indexing bool
	}{
		{name: "idle"}, {name: "indexing", indexing: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ui.GetLayout().Update(80, 24, false)
			libraries := make([]plex.LibraryInfo, 40)
			for i := range libraries {
				libraries[i] = plex.LibraryInfo{Title: fmt.Sprintf("Library %02d %s END", i, strings.Repeat("wide ", 12)), Count: i}
			}
			v := NewSearchStatusView(ui.PlexctlTheme, libraries)
			v.isIndexing = test.indexing
			v.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			initial := ansi.Strip(v.View())
			v.Update(ui.KeyPress("j"))
			if ansi.Strip(v.View()) == initial {
				t.Fatal("j did not scroll the status page")
			}
			v.Update(ui.KeyPress("k"))
			if ansi.Strip(v.View()) != initial {
				t.Fatal("k did not restore the previous status position")
			}
			v.Update(ui.KeyPress("down"))
			v.Update(ui.KeyPress("up"))
			if ansi.Strip(v.View()) != initial {
				t.Fatal("up/down differed from j/k")
			}
			for range 30 {
				v.Update(ui.KeyPress("l"))
			}
			if !strings.Contains(v.View(), "END") {
				t.Fatal("l did not reveal the end of a wide library name")
			}
			for range 30 {
				v.Update(ui.KeyPress("h"))
			}
			if ansi.Strip(v.View()) != initial {
				t.Fatal("h did not return to the first column")
			}
			v.Update(ui.KeyPress("end"))
			if !strings.Contains(v.View(), "Library 39") {
				t.Fatal("End did not reach the final library")
			}
			for range 30 {
				v.Update(ui.KeyPress("right"))
			}
			if !strings.Contains(v.View(), "39 items") {
				t.Fatal("Right did not reveal the final library count")
			}
			ui.GetLayout().Update(150, 70, false)
			v.Update(tea.WindowSizeMsg{Width: 150, Height: 70})
			view := v.View()
			if !strings.Contains(view, "SEARCH INDEX STATUS") || !strings.Contains(view, "Library 39") {
				t.Fatalf("resizing did not clamp scroll offsets to the visible content:\n%s", ansi.Strip(view))
			}
			if lipgloss.Width(view) > ui.GetLayout().InnerWidth() || lipgloss.Height(view) > ui.GetLayout().ContentHeight() {
				t.Fatal("status page overflowed the content pane")
			}
		})
	}
}

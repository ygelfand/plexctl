package navbar

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ygelfand/plexctl/internal/ui"
)

func TestKeyboardCursorRemainsVisibleInShortSidebar(t *testing.T) {
	items := make([]NavItem, 60)
	for i := range items {
		items[i] = NavItem{Title: fmt.Sprintf("Library %d", i)}
	}
	n := NewNavbar([]NavSection{{Title: "Libraries", Items: items}}, ui.PlexctlTheme)
	n.Height = 16
	n.SetFocused(true)
	for range 75 {
		n.MoveCursor(1)
		n.View()
		row := n.itemRows[n.Cursor()]
		if row.Y < 0 || row.Y >= n.actionTop || n.ItemAt(tea.MouseMsg{Y: row.Y}) != n.Cursor() {
			t.Fatalf("cursor %d is not visible or clickable", n.Cursor())
		}
	}
	if n.Cursor() != 59 || n.activeFlatIdx != 0 {
		t.Fatal("cursor did not stop at the last library independently of the active tab")
	}
	n.MoveCursor(-100)
	n.View()
	if n.Cursor() != 0 || n.itemRows[0].Y < 0 {
		t.Fatal("could not navigate back to the first library")
	}
}

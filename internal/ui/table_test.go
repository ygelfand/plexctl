package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestTableNavigationReachesClippedRowsAndColumns(t *testing.T) {
	for _, keys := range []struct{ name, left, down, up, right string }{
		{"vim", "h", "j", "k", "l"},
		{"arrows", "left", "down", "up", "right"},
	} {
		t.Run(keys.name, func(t *testing.T) {
			m := NewTable([]table.Column{{Title: "NAME", Width: 12}, {Title: "DETAIL", Width: 30}, {Title: "STATE", Width: 12}}, PlexctlTheme)
			rows := make([]table.Row, 30)
			for i := range rows {
				rows[i] = table.Row{fmt.Sprintf("Item %02d", i), "A long middle column", fmt.Sprintf("State %02d", i)}
			}
			m.SetRows(rows)
			m.SetHeight(6)
			m.SetWidth(20)
			for range 20 {
				m, _ = m.Update(KeyPress(keys.down))
			}
			m, _ = m.Update(KeyPress(keys.up))
			if m.Cursor() != 19 || !strings.Contains(m.View(), "Item 19") {
				t.Fatal("vertical navigation left the selected row outside the visible table")
			}
			for range 40 {
				m, _ = m.Update(KeyPress(keys.right))
			}
			view := ansi.Strip(m.View())
			if !strings.Contains(view, "STATE") || !strings.Contains(view, "State 19") || m.Cursor() != 19 {
				t.Fatalf("horizontal navigation did not reveal the last column of the selected row:\n%s", view)
			}
			if lipgloss.Width(view) > 20 || lipgloss.Height(view) > 6 {
				t.Fatal("table overflowed the content pane")
			}
			lines := strings.Split(view, "\n")
			last := strings.TrimSpace(lines[3])
			if !m.Click(click(15, 3)) || m.SelectedRow()[2] != last {
				t.Fatal("horizontal scrolling changed which row a click selected")
			}
			for range 40 {
				m, _ = m.Update(KeyPress(keys.left))
			}
			if !strings.Contains(m.View(), "NAME") || strings.Contains(m.View(), "STATE") {
				t.Fatal("left navigation did not return to the first column")
			}
			for range 40 {
				m, _ = m.Update(KeyPress(keys.right))
			}
			m.SetWidth(90)
			if view := m.View(); !strings.Contains(view, "NAME") || !strings.Contains(view, "STATE") {
				t.Fatal("widening the pane did not clamp horizontal scrolling")
			}
			m.SetRows(nil)
			m, _ = m.Update(KeyPress(keys.down))
			_ = m.View()
		})
	}
}

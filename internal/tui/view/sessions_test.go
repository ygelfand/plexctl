package view

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/ygelfand/plexctl/internal/ui"
)

func navigationRows(columns int) []table.Row {
	rows := make([]table.Row, 40)
	for i := range rows {
		for column := range columns {
			rows[i] = append(rows[i], fmt.Sprintf("R%02d C%d", i, column))
		}
	}
	return rows
}

func verifyTableNavigation(t *testing.T, model tea.Model, table *ui.Table) {
	t.Helper()
	ui.GetLayout().Update(80, 18, false)
	model.Update(tea.WindowSizeMsg{Width: 80, Height: 18})
	for range 20 {
		model.Update(ui.KeyPress("j"))
	}
	model.Update(ui.KeyPress("k"))
	if table.Cursor() != 19 {
		t.Fatal("j/k did not select the expected row")
	}
	left := ansi.Strip(model.View())
	for range 50 {
		model.Update(ui.KeyPress("l"))
	}
	view := ansi.Strip(model.View())
	row := table.SelectedRow()
	if !strings.Contains(view, row[len(row)-1]) || table.Cursor() != 19 || view == left {
		t.Fatalf("l did not reveal the selected row's final column:\n%s", view)
	}
	if lipgloss.Width(view) > ui.GetLayout().InnerWidth() || lipgloss.Height(view) > ui.GetLayout().ContentHeight() {
		t.Fatal("status table extends outside the content pane")
	}
	for range 50 {
		model.Update(ui.KeyPress("h"))
	}
	if ansi.Strip(model.View()) != left {
		t.Fatal("h did not restore the original columns and selection")
	}
}

func TestSessionsTableNavigation(t *testing.T) {
	tab := NewSessionsTab(ui.PlexctlTheme)
	tab.Update(DataLoadedMsg{owner: tab, data: navigationRows(len(tab.table.Columns()))})
	verifyTableNavigation(t, tab, &tab.table)
}

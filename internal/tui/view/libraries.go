package view

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/ui"
)

type LibrarySelectedMsg struct {
	ID    string
	Title string
}

type LibrariesTab struct {
	table  table.Model
	width  int
	height int
}

func NewLibrariesTab(theme tint.Tint) *LibrariesTab {
	columns := []table.Column{
		{Title: "ID", Width: 5},
		{Title: "TITLE", Width: 30},
		{Title: "TYPE", Width: 10},
		{Title: "AGENT", Width: 30},
	}

	return &LibrariesTab{
		table: ui.NewTable(columns, theme),
	}
}

func (t *LibrariesTab) Init() tea.Cmd {
	return t.fetchLibraries
}

func (t *LibrariesTab) Refresh() tea.Cmd {
	return t.fetchLibraries
}

func (t *LibrariesTab) fetchLibraries() tea.Msg {
	client, err := plex.NewClient()
	if err != nil {
		return err
	}

	res, err := client.SDK.Library.GetSections(context.Background())
	if err != nil {
		return err
	}

	if res.Object == nil || res.Object.MediaContainer == nil {
		return fmt.Errorf("no libraries found")
	}

	var rows []table.Row
	for _, dir := range res.Object.MediaContainer.Directory {
		id := ""
		if dir.Key != nil {
			id = *dir.Key
		}
		title := ""
		if dir.Title != nil {
			title = *dir.Title
		}
		agent := ""
		if dir.Agent != nil {
			agent = *dir.Agent
		}

		rows = append(rows, table.Row{
			id,
			title,
			string(dir.Type),
			agent,
		})
	}

	return rows
}

func (t *LibrariesTab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width = msg.Width
		t.height = msg.Height
		t.table.SetHeight(ui.GetTableHeight(t.height))
		t.table.SetWidth(t.width)
	case ui.ThemeChangedMsg:
		ui.UpdateTableTheme(&t.table, t.width, t.height)
	case []table.Row:
		t.table.SetRows(msg)
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			selected := t.table.SelectedRow()
			if len(selected) < 2 {
				return t, nil
			}
			return t, func() tea.Msg {
				return LibrarySelectedMsg{
					ID:    selected[0],
					Title: selected[1],
				}
			}
		case "r":
			return t, t.Refresh()
		}
	}
	t.table, cmd = t.table.Update(msg)
	return t, cmd
}

func (t *LibrariesTab) View() string {
	return t.table.View()
}

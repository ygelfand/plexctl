package view

import (
	"context"
	"fmt"

	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/ui"
)

type TasksTab struct {
	table  table.Model
	width  int
	height int
}

func NewTasksTab(theme tint.Tint) *TasksTab {
	columns := []table.Column{
		{Title: "NAME", Width: 20},
		{Title: "TITLE", Width: 30},
		{Title: "INTERVAL", Width: 10},
		{Title: "ENABLED", Width: 10},
	}

	return &TasksTab{
		table: ui.NewTable(columns, theme),
	}
}

func (t *TasksTab) Init() tea.Cmd {
	return t.fetchTasks
}

func (t *TasksTab) Refresh() tea.Cmd {
	return t.fetchTasks
}

func (t *TasksTab) fetchTasks() tea.Msg {
	client, err := plex.NewClient()
	if err != nil {
		return err
	}

	res, err := client.SDK.Butler.GetTasks(context.Background())
	if err != nil {
		return err
	}

	if res.Object == nil || res.Object.ButlerTasks == nil {
		return fmt.Errorf("no butler tasks found")
	}

	var rows []table.Row
	for _, task := range res.Object.ButlerTasks.ButlerTask {
		name := ""
		if task.Name != nil {
			name = *task.Name
		}

		title := ""
		if task.Title != nil {
			title = *task.Title
		}

		interval := ""
		if task.Interval != nil {
			interval = fmt.Sprintf("%d days", *task.Interval)
		}

		enabled := "false"
		if task.Enabled != nil && *task.Enabled {
			enabled = "true"
		}

		rows = append(rows, table.Row{
			name,
			title,
			interval,
			enabled,
		})
	}

	return rows
}

func (t *TasksTab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		case "s": // Start task
			selected := t.table.SelectedRow()
			if len(selected) > 0 {
				return t, t.startTask(selected[0])
			}
		case "x": // Stop task
			selected := t.table.SelectedRow()
			if len(selected) > 0 {
				return t, t.stopTask(selected[0])
			}
		case "r":
			return t, t.Refresh()
		}
	}

	t.table, cmd = t.table.Update(msg)
	return t, cmd
}

func (t *TasksTab) startTask(name string) tea.Cmd {
	return func() tea.Msg {
		client, err := plex.NewClient()
		if err != nil {
			return err
		}
		_, err = client.SDK.Butler.StartTask(context.Background(), operations.StartTaskRequest{
			ButlerTask: operations.PathParamButlerTask(name),
		})
		if err != nil {
			return err
		}
		return t.fetchTasks()
	}
}

func (t *TasksTab) stopTask(name string) tea.Cmd {
	return func() tea.Msg {
		client, err := plex.NewClient()
		if err != nil {
			return err
		}
		_, err = client.SDK.Butler.StopTask(context.Background(), operations.StopTaskRequest{
			ButlerTask: operations.ButlerTask(name),
		})
		if err != nil {
			return err
		}
		return t.fetchTasks()
	}
}

func (t *TasksTab) View() string {
	if len(t.table.Rows()) == 0 {
		return lipgloss.NewStyle().
			Padding(2).
			Foreground(ui.GetLayout().Theme().BrightBlack()).
			Render("No butler tasks found.")
	}
	return t.table.View()
}

func (t *TasksTab) HelpKeys() []ui.HelpKey {
	return []ui.HelpKey{
		{Key: "s", Desc: "Start Task"},
		{Key: "x", Desc: "Stop Task"},
		{Key: "j/up", Desc: "Move Up"},
		{Key: "k/down", Desc: "Move Down"},
	}
}

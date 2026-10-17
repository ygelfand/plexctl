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
	table   ui.Table
	width   int
	height  int
	request viewRequest
	err     error
}

func NewTasksTab(theme tint.Tint) *TasksTab {
	columns := []table.Column{
		{Title: "NAME", Width: 20},
		{Title: "TITLE", Width: 30},
		{Title: "INTERVAL", Width: 10},
		{Title: "ENABLED", Width: 10},
	}

	return &TasksTab{
		request: newViewRequest(),
		table:   ui.NewTable(columns, theme),
	}
}

func (t *TasksTab) Init() tea.Cmd {
	return t.Refresh()
}

func (t *TasksTab) Refresh() tea.Cmd {
	t.err = nil
	return t.request.start(t, fetchTasks)
}

func fetchTasks(ctx context.Context) (any, error) {
	client, err := plex.NewClientForContext(ctx)
	if err != nil {
		return nil, err
	}

	res, err := client.SDK.Butler.GetTasks(ctx)
	if err != nil {
		return nil, err
	}

	if res.Object == nil || res.Object.ButlerTasks == nil {
		return nil, fmt.Errorf("no butler tasks found")
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

	return rows, nil
}

func (t *TasksTab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width = ui.GetLayout().InnerWidth()
		t.height = msg.Height
		t.table.SetHeight(ui.GetTableHeight(t.height))
		t.table.SetWidth(t.width)
	case ui.ThemeChangedMsg:
		ui.UpdateTableTheme(&t.table, t.width, t.height)
	case DataLoadedMsg:
		if !t.request.accepts(t, msg) {
			return t, nil
		}
		t.err = msg.err
		if msg.err == nil {
			t.table.SetRows(msg.data.([]table.Row))
		}
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
	return t.request.start(t, func(ctx context.Context) (any, error) {
		client, err := plex.NewClientForContext(ctx)
		if err != nil {
			return nil, err
		}
		_, err = client.SDK.Butler.StartTask(ctx, operations.StartTaskRequest{
			ButlerTask: operations.PathParamButlerTask(name),
		})
		if err != nil {
			return nil, err
		}
		return fetchTasks(ctx)
	})
}

func (t *TasksTab) stopTask(name string) tea.Cmd {
	return t.request.start(t, func(ctx context.Context) (any, error) {
		client, err := plex.NewClientForContext(ctx)
		if err != nil {
			return nil, err
		}
		_, err = client.SDK.Butler.StopTask(ctx, operations.StopTaskRequest{
			ButlerTask: operations.ButlerTask(name),
		})
		if err != nil {
			return nil, err
		}
		return fetchTasks(ctx)
	})
}

func (t *TasksTab) View() string {
	if t.err != nil {
		return fmt.Sprintf("Unable to load tasks: %v\n[r] Retry", t.err)
	}
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
		{Key: "j/down", Desc: "Next Row"},
		{Key: "k/up", Desc: "Previous Row"},
		{Key: "h/left, l/right", Desc: "Scroll Columns Left / Right"},
	}
}

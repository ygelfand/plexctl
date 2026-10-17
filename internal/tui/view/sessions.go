package view

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/ui"
)

type SessionsTab struct {
	table   ui.Table
	width   int
	height  int
	request viewRequest
	err     error
}

func NewSessionsTab(theme tint.Tint) *SessionsTab {
	columns := []table.Column{
		{Title: "ID", Width: 10},
		{Title: "USER", Width: 15},
		{Title: "TITLE", Width: 40},
		{Title: "PLAYER", Width: 15},
		{Title: "STATE", Width: 10},
	}

	return &SessionsTab{
		request: newViewRequest(),
		table:   ui.NewTable(columns, theme),
	}
}

func (t *SessionsTab) Init() tea.Cmd {
	return t.Refresh()
}

func (t *SessionsTab) Refresh() tea.Cmd {
	t.err = nil
	return t.request.start(t, fetchSessions)
}

type sessionData struct {
	Title string `json:"title"`
	User  struct {
		Title string `json:"title"`
	} `json:"User"`
	Player struct {
		Title string `json:"title"`
		State string `json:"state"`
	} `json:"Player"`
	Session struct {
		ID string `json:"id"`
	} `json:"Session"`
}

type sessionsResponse struct {
	MediaContainer struct {
		Metadata []sessionData `json:"Metadata"`
	} `json:"MediaContainer"`
}

func fetchSessions(ctx context.Context) (any, error) {
	client, err := plex.NewClientForContext(ctx)
	if err != nil {
		return nil, err
	}

	res, err := client.SDK.Status.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := res.RawResponse.Body.Close(); err != nil {
			slog.Debug("Close sessions response", "error", err)
		}
	}()
	body, err := io.ReadAll(res.RawResponse.Body)
	if err != nil {
		return nil, err
	}

	var sRes sessionsResponse
	if err := json.Unmarshal(body, &sRes); err != nil {
		return nil, err
	}

	var rows []table.Row
	for _, session := range sRes.MediaContainer.Metadata {
		rows = append(rows, table.Row{
			session.Session.ID,
			session.User.Title,
			session.Title,
			session.Player.Title,
			session.Player.State,
		})
	}

	return rows, nil
}

func (t *SessionsTab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		case "s":
			selected := t.table.SelectedRow()
			if len(selected) > 0 {
				sessionID := selected[0]
				return t, t.stopSession(sessionID)
			}
		case "r":
			return t, t.Refresh()
		}
	}

	t.table, cmd = t.table.Update(msg)
	return t, cmd
}

func (t *SessionsTab) stopSession(id string) tea.Cmd {
	return t.request.start(t, func(ctx context.Context) (any, error) {
		client, err := plex.NewClientForContext(ctx)
		if err != nil {
			return nil, err
		}

		reason := "Terminated via plexctl TUI"
		_, err = client.SDK.Status.TerminateSession(ctx, operations.TerminateSessionRequest{
			SessionID: id,
			Reason:    &reason,
		})
		if err != nil {
			return nil, err
		}

		return fetchSessions(ctx)
	})
}

func (t *SessionsTab) View() string {
	if t.err != nil {
		return fmt.Sprintf("Unable to load sessions: %v\n[r] Retry", t.err)
	}
	if len(t.table.Rows()) == 0 {
		return lipgloss.NewStyle().
			Padding(2).
			Foreground(ui.GetLayout().Theme().BrightBlack()).
			Render("No active sessions found.")
	}
	return t.table.View()
}

func (t *SessionsTab) HelpKeys() []ui.HelpKey {
	return []ui.HelpKey{
		{Key: "s", Desc: "Stop Session"},
		{Key: "j/down", Desc: "Next Row"},
		{Key: "k/up", Desc: "Previous Row"},
		{Key: "h/left, l/right", Desc: "Scroll Columns Left / Right"},
	}
}

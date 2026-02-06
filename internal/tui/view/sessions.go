package view

import (
	"context"
	"encoding/json"
	"io"

	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/ui"
)

type SessionsTab struct {
	table  table.Model
	width  int
	height int
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
		table: ui.NewTable(columns, theme),
	}
}

func (t *SessionsTab) Init() tea.Cmd {
	return t.fetchSessions
}

func (t *SessionsTab) Refresh() tea.Cmd {
	return t.fetchSessions
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

func (t *SessionsTab) fetchSessions() tea.Msg {
	client, err := plex.NewClient()
	if err != nil {
		return err
	}

	res, err := client.SDK.Status.ListSessions(context.Background())
	if err != nil {
		return err
	}

	defer res.RawResponse.Body.Close()
	body, err := io.ReadAll(res.RawResponse.Body)
	if err != nil {
		return err
	}

	var sRes sessionsResponse
	if err := json.Unmarshal(body, &sRes); err != nil {
		return err
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

	if len(rows) == 0 {
		return []table.Row{}
	}

	return rows
}

func (t *SessionsTab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
	return func() tea.Msg {
		client, err := plex.NewClient()
		if err != nil {
			return err
		}

		reason := "Terminated via plexctl TUI"
		_, err = client.SDK.Status.TerminateSession(context.Background(), operations.TerminateSessionRequest{
			SessionID: id,
			Reason:    &reason,
		})
		if err != nil {
			return err
		}

		return t.fetchSessions()
	}
}

func (t *SessionsTab) View() string {
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
		{Key: "j/up", Desc: "Move Up"},
		{Key: "k/down", Desc: "Move Down"},
	}
}

package view

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/charmbracelet/bubbles/table"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/ui"
)

func TestTableResultsStayWithTheirViewAndRefresh(t *testing.T) {
	sessions := NewSessionsTab(ui.PlexctlTheme)
	tasks := NewTasksTab(ui.PlexctlTheme)
	oldSessions := NewSessionsTab(ui.PlexctlTheme)
	sessions.Refresh()
	oldID := sessions.request.id
	sessions.Refresh()
	result := DataLoadedMsg{owner: sessions, id: sessions.request.id, data: []table.Row{{"session", "user", "movie", "player", "playing"}}}
	tasks.Update(result)
	oldSessions.Update(result)
	if len(tasks.table.Rows()) != 0 || len(oldSessions.table.Rows()) != 0 {
		t.Fatal("session data was accepted by another tab or view instance")
	}
	sessions.Update(result)
	result.id, result.data = oldID, []table.Row{{"stale", "", "", "", ""}}
	sessions.Update(result)
	if got := sessions.table.Rows()[0][0]; got != "session" {
		t.Fatalf("old refresh replaced newer rows: %q", got)
	}
}

func TestHomeRejectsPreviousViewAndShowsLoadErrors(t *testing.T) {
	old := NewHomeView(ui.PlexctlTheme)
	current := NewHomeView(ui.PlexctlTheme)
	current.Update(DataLoadedMsg{owner: old, data: []components.Hub{{Title: ui.Ptr("Private hub")}}})
	if len(current.hubs) != 0 {
		t.Fatal("new Home accepted the old user's hubs")
	}
	current.Update(DataLoadedMsg{owner: current, err: errors.New("unavailable")})
	if current.loading || current.err == nil {
		t.Fatal("failed Home request stayed stuck in loading state")
	}
}

func TestViewRequestRetainsServerWhenCommandIsDelayed(t *testing.T) {
	view := NewSessionsTab(ui.PlexctlTheme)
	view.request.server = config.ServerTarget{ID: "original", Token: "original-user"}
	cmd := view.request.start(view, func(ctx context.Context) (any, error) {
		return config.ServerFromContext(ctx), nil
	})
	view.request.server = config.ServerTarget{ID: "replacement", Token: "new-user"}
	result := cmd().(DataLoadedMsg)
	if target := result.data.(config.ServerTarget); target.ID != "original" || target.Token != "original-user" {
		t.Fatal("delayed request used replacement credentials")
	}
}

func TestHistoryDoesNotDuplicatePendingPageOrAcceptOldCursor(t *testing.T) {
	history := NewHistoryTab(ui.PlexctlTheme)
	if history.Init() == nil || history.Init() != nil {
		t.Fatal("History did not retain its pending request across tab activation")
	}
	oldID := history.request.id
	history.Refresh()
	cursor := history.dayCursor
	history.Update(DataLoadedMsg{owner: history, id: oldID, data: historyResult{nextDay: cursor.Add(-24 * time.Hour)}})
	if !history.dayCursor.Equal(cursor) || !history.isLoading {
		t.Fatal("old page changed the refreshed History cursor or loading state")
	}
	history.Update(DataLoadedMsg{owner: history, id: history.request.id, err: errors.New("unavailable")})
	if history.isLoading || history.err == nil || !history.dayCursor.Equal(cursor) {
		t.Fatal("failed page skipped history or left loading stuck")
	}
}

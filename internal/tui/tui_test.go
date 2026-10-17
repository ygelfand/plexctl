package tui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/LukeHagar/plexgo/models/operations"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/tui/player"
	"github.com/ygelfand/plexctl/internal/tui/view"
	"github.com/ygelfand/plexctl/internal/tui/widget/poster"
	"github.com/ygelfand/plexctl/internal/ui"
	"go.dalton.dog/bubbleup"
)

func TestLateSessionResponseUpdatesItsInactiveTab(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status/sessions" {
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"MediaContainer":{"Metadata":[{"title":"Movie","User":{"title":"owner"},"Player":{"title":"mpv","state":"playing"},"Session":{"id":"session-id"}}]}}`)
	}))
	defer server.Close()
	cfg := config.Get()
	old := *cfg
	t.Cleanup(func() { *cfg = old })
	t.Setenv("PLEXCTL_TOKEN", "test-token")
	cfg.DefaultServer = "session-test"
	cfg.Servers = map[string]config.Server{"session-test": {URL: server.URL}}
	sessions, tasks := view.NewSessionsTab(ui.PlexctlTheme), view.NewTasksTab(ui.PlexctlTheme)
	c := focusController()
	c.tabManager.tabModels = []tea.Model{sessions, tasks}
	ui.GetLayout().Update(160, 30, false)
	sessions.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	tasks.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	request := sessions.Init()
	c.tabManager.activeTabIdx = 1
	c.Update(request())
	if !strings.Contains(sessions.View(), "owner") {
		t.Fatalf("inactive Sessions did not receive its response: %s", sessions.View())
	}
	if strings.Contains(tasks.View(), "owner") || strings.Contains(tasks.View(), "session-id") {
		t.Fatal("session response appeared in Tasks")
	}
}

func TestSupersededUserSwitchAndReloadCannotReplaceCurrentProfile(t *testing.T) {
	c := focusController()
	c.userSwitchID = 2
	c.reloadID = 2
	c.data = plex.LoaderResult{ServerID: "current"}
	original := config.Get().HomeUser
	c.Update(switchUserSuccessMsg{id: 1, homeUser: config.HomeUser{AuthToken: "stale"}})
	c.Update(reloadedDataMsg{id: 1, data: plex.LoaderResult{ServerID: "stale"}})
	if config.Get().HomeUser != original || c.data.ServerID != "current" {
		t.Fatal("a superseded user switch changed the current profile")
	}
	c.Update(switchUserSuccessMsg{id: 2, owner: &mouseOverlay{}, homeUser: config.HomeUser{AuthToken: "closed-picker"}})
	if config.Get().HomeUser != original {
		t.Fatal("a closed user picker changed the current profile")
	}
}

func TestSupersededUserPickerResponseDoesNotReopenOverlay(t *testing.T) {
	c := focusController()
	c.userPickerID = 2
	c.Update(usersLoadedMsg{id: 1, users: []operations.HomeUser{{Title: "Old user"}, {Title: "Another user"}}})
	if c.navigator.ActiveOverlay() != nil {
		t.Fatal("superseded user discovery opened an overlay")
	}
}

func TestLateNavigationCannotOpenPreviousProfilesMedia(t *testing.T) {
	c := focusController()
	previous := config.ServerTarget{ID: "previous-server", Token: "previous-user"}
	if cmd := c.handleSelectMedia(ui.SelectMediaMsg{Server: previous, RatingKey: "1"}); cmd != nil {
		t.Fatal("stale selection started a metadata request")
	}
	c.tabManager.tabModels[1] = view.NewMediaView("1", "Movies", ui.PlexctlTheme)
	c.Update(ui.JumpToDetailMsg{Server: previous, SectionID: "1", RatingKey: "1", Type: "movie"})
	if c.tabManager.activeTabIdx != 0 {
		t.Fatal("late metadata result navigated into the replacement profile")
	}
}

func TestPosterNavigationKeysReachContent(t *testing.T) {
	for _, tt := range []struct {
		name string
		tab  int
	}{
		{name: "home", tab: 0},
		{name: "movies", tab: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := focusController()
			c.alert = *bubbleup.NewAlertModel(40, true, time.Second)
			c.tabManager.SetActive(tt.tab)
			grid := poster.NewPosterGrid(make([]components.Metadata, 12), ui.PlexctlTheme)
			grid.SetSize(3*ui.PosterStepWidth, 2*ui.PosterTotalHeight)
			grid.SetCursor(1)
			c.tabManager.tabModels[tt.tab] = grid
			for _, step := range []struct {
				key  string
				want int
			}{
				{key: "l", want: 2}, {key: "h", want: 1},
				{key: "j", want: 4}, {key: "down", want: 7},
				{key: "k", want: 4}, {key: "up", want: 1},
				{key: "left", want: 0}, {key: "right", want: 1},
				{key: "h", want: 0}, {key: "h", want: 0},
			} {
				c.Update(ui.KeyPress(step.key))
				if grid.Cursor != step.want || c.tabManager.activeTabIdx != tt.tab {
					t.Fatalf("%s selected poster %d in tab %d; want poster %d in tab %d",
						step.key, grid.Cursor, c.tabManager.activeTabIdx, step.want, tt.tab)
				}
			}
		})
	}
}

func TestHomeShortcutFocusesHome(t *testing.T) {
	for _, tt := range []struct {
		name  string
		focus paneFocus
	}{
		{name: "content", focus: paneContent},
		{name: "sidebar", focus: paneSidebar},
		{name: "player", focus: panePlayer},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := focusController()
			c.alert = *bubbleup.NewAlertModel(40, true, time.Second)
			c.playerStatus = player.PlayerStatus{State: operations.StatePlaying}
			c.tabManager.SetActive(1)
			c.focusPane(tt.focus)
			c.Update(ui.KeyPress("H"))
			if c.tabManager.activeTabIdx != 0 || c.focus != paneContent {
				t.Fatal("H did not open Home and focus its content")
			}
		})
	}
}

func TestRightClickNavigatesBackFromAnotherPane(t *testing.T) {
	for _, tt := range []struct {
		name  string
		focus paneFocus
	}{
		{name: "sidebar", focus: paneSidebar},
		{name: "player", focus: panePlayer},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := focusController()
			c.alert = *bubbleup.NewAlertModel(40, true, time.Second)
			c.playerStatus = player.PlayerStatus{State: operations.StatePlaying}
			c.focusPane(tt.focus)
			ui.GetLayout().Update(100, 40, true)
			content := c.tabManager.ActiveModel().(*mouseOverlay)
			c.Update(tea.MouseMsg{
				X: ui.SidebarTotalWidth + 4, Y: 2,
				Button: tea.MouseButtonRight, Action: tea.MouseActionPress,
			})
			key, ok := content.last.(tea.KeyMsg)
			if !ok || key.Type != tea.KeyEsc {
				t.Fatal("right click did not send Back to the content pane")
			}
			if c.focus != paneContent {
				t.Fatal("right click did not restore content focus")
			}
		})
	}
}

type letterTab struct {
	mouseOverlay
	letter string
}

func (m *letterTab) CanJumpLetters() bool { return true }
func (m *letterTab) JumpToLetter(letter string) tea.Cmd {
	m.letter = letter
	return nil
}

func TestNavigationFocusesItsDestination(t *testing.T) {
	for _, tt := range []struct {
		name string
		msg  tea.Msg
		want int
	}{
		{name: "next library", msg: tea.KeyMsg{Type: tea.KeyTab}, want: 1},
		{name: "previous library", msg: tea.KeyMsg{Type: tea.KeyShiftTab}, want: 2},
		{name: "search result", msg: ui.JumpToDetailMsg{SectionID: "music", RatingKey: "500", Type: "artist"}, want: 2},
		{name: "detail in current library", msg: ui.JumpToDetailMsg{RatingKey: "500", Type: "artist"}, want: 2},
		{name: "alphabet destination", msg: ui.JumpToLetterMsg{Letter: "Z"}, want: 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := focusController()
			c.tabManager.tabModels[2] = view.NewMediaView("music", "Music", ui.PlexctlTheme)
			if tt.name == "detail in current library" {
				c.tabManager.SetActive(2)
			}
			letter := &letterTab{}
			if tt.name == "alphabet destination" {
				c.tabManager.tabModels[0] = letter
			}
			c.focusPane(paneSidebar)
			c.Update(tt.msg)
			if c.focus != paneContent || c.tabManager.activeTabIdx != tt.want {
				t.Fatal("navigation left keyboard focus outside the destination library")
			}
			if tt.name == "alphabet destination" && letter.letter != "Z" {
				t.Fatal("alphabet jump did not reach the destination")
			}
		})
	}
}

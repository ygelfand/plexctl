package tui

import (
	"strings"
	"testing"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/LukeHagar/plexgo/models/operations"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ygelfand/plexctl/internal/tui/player"
	"github.com/ygelfand/plexctl/internal/tui/widget/navbar"
	"github.com/ygelfand/plexctl/internal/ui"
)

func focusController() *Controller {
	nav := navbar.NewNavbar([]navbar.NavSection{{Items: []navbar.NavItem{
		{Title: "Home"}, {Title: "Movies"}, {Title: "Music"},
	}}}, ui.PlexctlTheme)
	return &Controller{
		theme: ui.PlexctlTheme,
		tabManager: &TabManager{
			navbar: nav, tabModels: []tea.Model{&mouseOverlay{}, &mouseOverlay{}, &mouseOverlay{}},
		},
		navigator: NewNavigator(ui.PlexctlTheme),
	}
}

func TestDirectionalPaneFocusAndKeyRepeat(t *testing.T) {
	c := focusController()
	c.playerStatus = player.PlayerStatus{State: operations.StatePlaying, AudioOnly: true}
	for _, step := range []struct {
		key  tea.KeyType
		pane paneFocus
	}{
		{tea.KeyCtrlH, paneSidebar}, {tea.KeyCtrlL, paneContent},
		{tea.KeyCtrlJ, panePlayer}, {tea.KeyCtrlK, paneContent},
		{tea.KeyCtrlJ, panePlayer}, {tea.KeyCtrlH, paneSidebar},
		{tea.KeyCtrlJ, paneSidebar}, {tea.KeyCtrlK, paneSidebar},
		{tea.KeyCtrlL, paneContent},
	} {
		for range 8 {
			c.Update(tea.KeyMsg{Type: step.key})
			if c.focus != step.pane {
				t.Fatalf("%v focused %v, want %v", step.key, c.focus, step.pane)
			}
		}
	}
	if c.navigator.ActiveOverlay() != nil {
		t.Fatal("pane navigation opened a dialog")
	}
	c.playerStatus = player.PlayerStatus{}
	c.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if c.focus != paneContent {
		t.Fatal("focused a missing player")
	}
}

func TestSidebarSelectionDoesNotLeakIntoContent(t *testing.T) {
	c := focusController()
	content := c.tabManager.ActiveModel().(*mouseOverlay)
	c.Update(tea.KeyMsg{Type: tea.KeyCtrlH})
	c.Update(ui.KeyPress("j"))
	c.Update(ui.KeyPress("down"))
	c.Update(ui.KeyPress("k"))
	if c.tabManager.navbar.Cursor() != 1 || c.tabManager.activeTabIdx != 0 || content.last != nil {
		t.Fatal("sidebar navigation changed content before activation")
	}
	c.Update(ui.KeyPress("enter"))
	if c.focus != paneContent || c.tabManager.activeTabIdx != 1 {
		t.Fatal("Enter did not open the selected library and focus it")
	}
	c.Update(tea.KeyMsg{Type: tea.KeyCtrlH})
	if c.tabManager.navbar.Cursor() != 1 {
		t.Fatal("returning to navigation lost the active library")
	}
}

func TestPaneKeysRespectDialogsAndMouseFocus(t *testing.T) {
	c := focusController()
	overlay := &mouseOverlay{}
	c.navigator.Push(overlay)
	c.Update(tea.KeyMsg{Type: tea.KeyCtrlH})
	if c.focus != paneContent || overlay.last.(tea.KeyMsg).Type != tea.KeyCtrlH {
		t.Fatal("pane key escaped a modal text field")
	}
	c.navigator.Pop()
	ui.GetLayout().Update(100, 40, false)
	c.renderBaseView()
	c.handleMouse(tea.MouseMsg{X: 4, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if c.focus != paneSidebar {
		t.Fatal("sidebar click did not focus navigation")
	}
	c.handleMouse(tea.MouseMsg{
		X: ui.SidebarTotalWidth + 4, Y: 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	})
	if c.focus != paneContent {
		t.Fatal("content click did not restore keyboard focus")
	}
}

type alphabetTab struct {
	letterTab
	rail *ui.AlphabetRail
	meta components.Metadata
}

func (m *alphabetTab) AlphabetRail() *ui.AlphabetRail            { return m.rail }
func (m *alphabetTab) GetSelectedMetadata() *components.Metadata { return &m.meta }
func (m *alphabetTab) View() string {
	if m.rail == nil {
		return ""
	}
	return m.rail.View(ui.AlphabetLetter(m.meta), ui.PlexctlTheme)
}

func alphabetFocusController() (*Controller, *alphabetTab) {
	c := focusController()
	rail := ui.NewAlphabetRail(50, 0, 27)
	tab := &alphabetTab{
		rail: &rail,
		meta: components.Metadata{Title: "The Matrix", TitleSort: ui.Ptr("Matrix")},
	}
	c.tabManager.tabModels[0] = tab
	return c, tab
}

func TestAlphabetFocusRequiresEnterToJump(t *testing.T) {
	c, tab := alphabetFocusController()
	c.Update(tea.KeyMsg{Type: tea.KeyCtrlH})
	c.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	if c.focus != paneContent {
		t.Fatal("Ctrl+L from sidebar skipped content")
	}
	c.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	if c.focus != paneAlphabet || !tab.rail.Focused || !strings.Contains(tab.View(), "[M]") {
		t.Fatal("Ctrl+L did not focus the rail at the selected title's sort letter")
	}
	for _, step := range []struct{ key, letter string }{
		{"j", "N"}, {"down", "O"}, {"k", "N"}, {"up", "M"},
		{"home", "#"}, {"up", "#"}, {"end", "Z"}, {"j", "Z"}, {"k", "Y"},
		{"l", "Y"}, {"right", "Y"},
	} {
		c.Update(ui.KeyPress(step.key))
		if got := tab.rail.SelectedLetter(); got != step.letter {
			t.Fatalf("%s selected %q, want %q", step.key, got, step.letter)
		}
		if tab.letter != "" || tab.last != nil {
			t.Fatalf("%s changed content before Enter", step.key)
		}
	}
	for range 8 {
		c.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	}
	if c.focus != paneAlphabet || tab.rail.SelectedLetter() != "Y" {
		t.Fatal("holding Ctrl+L reset the letter or moved beyond the rail")
	}
	c.Update(ui.KeyPress("enter"))
	if tab.letter != "Y" || tab.last != nil || c.focus != paneContent || tab.rail.Focused {
		t.Fatal("Enter did not jump once and return focus to content")
	}
}

func TestLeavingAlphabetCancelsSelection(t *testing.T) {
	for _, key := range []tea.KeyMsg{
		ui.KeyPress("esc"), ui.KeyPress("h"), ui.KeyPress("left"), {Type: tea.KeyCtrlH},
	} {
		t.Run(key.String(), func(t *testing.T) {
			c, tab := alphabetFocusController()
			c.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
			c.Update(ui.KeyPress("j"))
			c.Update(key)
			if c.focus != paneContent || tab.rail.Focused || tab.letter != "" || tab.last != nil {
				t.Fatal("leaving the rail jumped or sent a key to content")
			}
			c.Update(tea.KeyMsg{Type: tea.KeyCtrlH})
			if c.focus != paneSidebar {
				t.Fatal("Ctrl+H from content did not continue left to the sidebar")
			}
		})
	}
}

func TestAlphabetFocusFollowsVisibleRail(t *testing.T) {
	for _, test := range []struct {
		name string
		hide func(*Controller, *alphabetTab)
	}{
		{"resize", func(_ *Controller, tab *alphabetTab) { tab.rail.SetBounds(8, 0, 3) }},
		{"detail without rail", func(_ *Controller, tab *alphabetTab) { tab.rail = nil }},
		{"replacement tab", func(c *Controller, _ *alphabetTab) { c.tabManager.tabModels[0] = &mouseOverlay{} }},
		{"replacement rail", func(_ *Controller, tab *alphabetTab) {
			rail := ui.NewAlphabetRail(50, 0, 27)
			tab.rail = &rail
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, tab := alphabetFocusController()
			c.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
			old := tab.rail
			test.hide(c, tab)
			ui.GetLayout().Update(100, 40, false)
			c.renderBaseView()
			if c.focus != paneContent || c.focusedAlphabet != nil || old.Focused {
				t.Fatal("focus remained on a hidden or replaced rail")
			}
		})
	}
}

func TestAlphabetFocusRespectsDialogsAndPlayer(t *testing.T) {
	c, tab := alphabetFocusController()
	c.Update(tea.KeyMsg{Type: tea.KeyCtrlL})
	overlay := &mouseOverlay{}
	c.navigator.Push(overlay)
	c.Update(ui.KeyPress("j"))
	if tab.rail.SelectedLetter() != "M" || overlay.last.(tea.KeyMsg).String() != "j" {
		t.Fatal("dialog input moved the rail behind it")
	}
	c.navigator.Pop()
	c.playerStatus = player.PlayerStatus{State: operations.StatePlaying, AudioOnly: true}
	c.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if c.focus != panePlayer || tab.rail.Focused {
		t.Fatal("Ctrl+J did not move from the rail to the active player")
	}
	c.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	if c.focus != paneContent {
		t.Fatal("Ctrl+K did not return to content")
	}
}

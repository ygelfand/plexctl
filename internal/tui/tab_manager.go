package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/tui/view"
	"github.com/ygelfand/plexctl/internal/tui/widget/navbar"
	"github.com/ygelfand/plexctl/internal/ui"
)

type TabManager struct {
	tabModels    []tea.Model
	navbar       *navbar.Navbar
	activeTabIdx int
	libCount     int
	sessionsIdx  int
	historyIdx   int
	tasksIdx     int
	searchIdx    int
}

func NewTabManager(data plex.LoaderResult, theme tint.Tint, sidebarWidth int) *TabManager {
	tm := &TabManager{}
	tm.setupTabs(data, theme, sidebarWidth)
	return tm
}

func (tm *TabManager) setupTabs(data plex.LoaderResult, theme tint.Tint, sidebarWidth int) tea.Cmd {
	cfg := config.Get()
	_, server, _ := cfg.GetActiveServer()

	var newModels []tea.Model
	var cmds []tea.Cmd

	// Home
	m := view.NewHomeView(theme)
	newModels = append(newModels, m)
	if tm.activeTabIdx == 0 {
		cmds = append(cmds, m.Init())
	}

	// Libraries
	libItems := []navbar.NavItem{{ID: "home", Title: "Home", Type: "home"}}
	customIcons := make(map[string]string)

	for _, lib := range data.Libraries {
		opts, ok := server.Libraries.Settings[lib.ID]
		if ok && opts.Hidden {
			continue
		}

		libItems = append(libItems, navbar.NavItem{ID: lib.ID, Title: lib.Title, Type: lib.Type})
		if icon := opts.GetIcon(cfg.IconType); icon != "" {
			customIcons[lib.ID] = icon
		}

		m := view.NewMediaView(lib.ID, lib.Title, theme)
		newModels = append(newModels, m)
		if len(newModels)-1 == tm.activeTabIdx {
			cmds = append(cmds, m.Init())
		}
	}

	tm.libCount = len(newModels)

	// Status Section
	tm.sessionsIdx = len(newModels)
	tm.historyIdx = len(newModels) + 1
	tm.tasksIdx = len(newModels) + 2

	statusItems := []navbar.NavItem{
		{ID: "sessions", Title: "Sessions", Type: "sessions"},
		{ID: "history", Title: "History", Type: "history"},
		{ID: "tasks", Title: "Tasks", Type: "tasks"},
	}

	for _, st := range []string{"sessions", "history", "tasks"} {
		var m tea.Model
		switch st {
		case "sessions":
			m = view.NewSessionsTab(theme)
		case "history":
			m = view.NewHistoryTab(theme)
		case "tasks":
			m = view.NewTasksTab(theme)
		}
		newModels = append(newModels, m)
		if len(newModels)-1 == tm.activeTabIdx {
			cmds = append(cmds, m.Init())
		}
	}

	// Search Section
	tm.searchIdx = len(newModels)
	searchItems := []navbar.NavItem{
		{ID: "search_status", Title: "Status", Type: "search_status"},
	}
	sm := view.NewSearchStatusView(theme, data.Libraries)
	newModels = append(newModels, sm)
	if len(newModels)-1 == tm.activeTabIdx {
		cmds = append(cmds, sm.Init())
	}

	sections := []navbar.NavSection{
		{Title: "Libraries", Items: libItems},
		{Title: "Status", Items: statusItems},
		{Title: "Search", Items: searchItems},
	}

	tm.tabModels = newModels
	tm.navbar = navbar.NewNavbar(sections, theme)
	tm.navbar.SetCustomIcons(customIcons)
	tm.navbar.Width = sidebarWidth

	// Reset index if out of bounds (due to changing library count)
	if tm.activeTabIdx >= len(tm.tabModels) {
		tm.activeTabIdx = 0
	}
	tm.navbar.SetActive(tm.activeTabIdx)

	// Propagate dimensions
	layout := ui.GetLayout()
	if layout.TotalWidth() > 0 {
		cmds = append(cmds, func() tea.Msg {
			return tea.WindowSizeMsg{Width: layout.TotalWidth(), Height: layout.TotalHeight()}
		})
	}

	return tea.Batch(cmds...)
}

func (tm *TabManager) ActiveModel() tea.Model {
	if tm.activeTabIdx >= 0 && tm.activeTabIdx < len(tm.tabModels) {
		return tm.tabModels[tm.activeTabIdx]
	}
	return nil
}

func (tm *TabManager) SetActive(idx int) tea.Cmd {
	if idx < 0 || idx >= len(tm.tabModels) {
		return nil
	}
	tm.activeTabIdx = idx
	tm.navbar.SetActive(tm.activeTabIdx)
	if tm.tabModels[tm.activeTabIdx] != nil {
		return tm.tabModels[tm.activeTabIdx].Init()
	}
	return nil
}

func (tm *TabManager) NextTab() tea.Cmd {
	return tm.SetActive((tm.activeTabIdx + 1) % len(tm.tabModels))
}

func (tm *TabManager) PrevTab() tea.Cmd {
	return tm.SetActive((tm.activeTabIdx - 1 + len(tm.tabModels)) % len(tm.tabModels))
}

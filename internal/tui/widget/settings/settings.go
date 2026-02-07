package settings

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/ui"
)

type SettingsFinishedMsg struct {
	Config *config.Config
}

type settingItem struct {
	id          string
	title       string
	description string
	current     string
}

func (i settingItem) Title() string       { return i.title }
func (i settingItem) Description() string { return i.description + " (Current: " + i.current + ")" }
func (i settingItem) FilterValue() string { return i.title }

type selectionItem struct {
	id    string
	value string
}

func (i selectionItem) Title() string       { return i.value }
func (i selectionItem) Description() string { return "" }
func (i selectionItem) FilterValue() string { return i.value }

type SettingsOverlayModel struct {
	list          list.Model
	selectionList list.Model
	width, height int
	theme         tint.Tint
	tints         []tint.Tint
	isSelecting   bool
	activeSetting string
}

func NewSettingsOverlayModel(theme tint.Tint) *SettingsOverlayModel {
	l := list.New(nil, list.NewDefaultDelegate(), 68, 20)
	l.Title = "Global Settings"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.KeyMap.Quit.SetKeys("q")

	s := list.New(nil, list.NewDefaultDelegate(), 68, 20)
	s.SetShowStatusBar(false)
	s.SetFilteringEnabled(false)

	m := &SettingsOverlayModel{
		list:          l,
		selectionList: s,
		theme:         theme,
		tints:         append([]tint.Tint{ui.PlexctlTheme}, tint.DefaultTints()...),
	}
	m.updateItems()
	return m
}

func (m *SettingsOverlayModel) updateItems() {
	cfg := config.Get()
	items := []list.Item{
		settingItem{id: "theme", title: "Theme", description: "UI color scheme", current: cfg.Theme},
		settingItem{id: "icon_type", title: "Icon Mode", description: "Icon set for navigation", current: string(cfg.IconType)},
		settingItem{id: "name_format", title: "Name Format", description: "How libraries are named", current: string(cfg.LibraryNameFormat)},
		settingItem{id: "default_view_mode", title: "Default View Mode", description: "Initial view for libraries", current: string(cfg.DefaultViewMode)},
		settingItem{id: "default_to_tui", title: "Default to TUI", description: "Start TUI if no command given", current: fmt.Sprintf("%v", cfg.DefaultToTui)},
		settingItem{id: "cache", title: "Enable Cache", description: "Cache Plex data locally", current: fmt.Sprintf("%v", !cfg.NoCache)},
		settingItem{id: "auto_home_login", title: "Auto Home Login", description: "Login automatically if token exists", current: fmt.Sprintf("%v", cfg.AutoHomeLogin)},
		settingItem{id: "close_video_on_quit", title: "Close Video On Quit", description: "Close mpv when exiting app", current: fmt.Sprintf("%v", cfg.CloseVideoOnQuit)},
	}
	m.list.SetItems(items)
}

func (m *SettingsOverlayModel) Init() tea.Cmd {
	return nil
}

func (m *SettingsOverlayModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		listW := min(m.width-2, 88)
		listH := min(m.height-10, 30)
		m.list.SetSize(listW, listH)
		m.selectionList.SetSize(listW, listH)
	}

	if m.isSelecting {
		if msg, ok := msg.(tea.KeyMsg); ok && msg.String() == "esc" {
			m.isSelecting = false
			return m, nil
		}

		oldIndex := m.selectionList.Index()
		m.selectionList, cmd = m.selectionList.Update(msg)
		newIndex := m.selectionList.Index()

		if m.activeSetting == "theme" && oldIndex != newIndex {
			selected := m.tints[newIndex]
			m.theme = selected
			return m, tea.Batch(cmd, func() tea.Msg { return ui.ThemeChangedMsg{Theme: selected} })
		}

		if msg, ok := msg.(tea.KeyMsg); ok && msg.String() == "enter" {
			selected := m.selectionList.SelectedItem().(selectionItem)
			m.applySetting(m.activeSetting, selected.id)
			m.isSelecting = false
			m.updateItems()
		}
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "s":
			return nil, func() tea.Msg { return SettingsFinishedMsg{Config: config.Get()} }
		case "enter":
			if item, ok := m.list.SelectedItem().(settingItem); ok {
				if m.handleToggle(item.id) {
					return m, nil
				}
				m.prepareSelection(item.id)
				m.isSelecting = true
			}
		}
	}

	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *SettingsOverlayModel) handleToggle(id string) bool {
	cfg := config.Get()
	handled := true

	switch id {
	case "cache":
		cfg.NoCache = !cfg.NoCache
	case "auto_home_login":
		cfg.AutoHomeLogin = !cfg.AutoHomeLogin
	case "default_to_tui":
		cfg.DefaultToTui = !cfg.DefaultToTui
	case "close_video_on_quit":
		cfg.CloseVideoOnQuit = !cfg.CloseVideoOnQuit
	default:
		handled = false
	}

	if handled {
		_ = cfg.Save()
		m.updateItems()
	}

	return handled
}

func (m *SettingsOverlayModel) prepareSelection(id string) {
	m.activeSetting = id
	var items []list.Item

	switch id {
	case "theme":
		m.selectionList.Title = "Choose Theme"
		for _, t := range m.tints {
			items = append(items, selectionItem{id: t.ID(), value: t.ID()})
		}
	case "icon_type":
		m.selectionList.Title = "Choose Icon Mode"
		items = []list.Item{
			selectionItem{id: string(config.IconTypeEmoji), value: "Emoji"},
			selectionItem{id: string(config.IconTypeNerdFonts), value: "Nerd Fonts"},
			selectionItem{id: string(config.IconTypeASCII), value: "ASCII"},
		}
	case "name_format":
		m.selectionList.Title = "Choose Name Format"
		items = []list.Item{
			selectionItem{id: string(config.LibraryNameFormatIconName), value: "Icon + Name"},
			selectionItem{id: string(config.LibraryNameFormatNameIcon), value: "Name + Icon"},
			selectionItem{id: string(config.LibraryNameFormatIconOnly), value: "Icon Only"},
			selectionItem{id: string(config.LibraryNameFormatName), value: "Name Only"},
		}
	case "default_view_mode":
		m.selectionList.Title = "Choose Default View Mode"
		items = []list.Item{
			selectionItem{id: string(config.ViewModeList), value: "List"},
			selectionItem{id: string(config.ViewModePoster), value: "Poster"},
		}
	}

	m.selectionList.SetItems(items)
	// Find current and select it
	cfg := config.Get()
	current := ""
	switch id {
	case "theme":
		current = cfg.Theme
	case "icon_type":
		current = string(cfg.IconType)
	case "name_format":
		current = string(cfg.LibraryNameFormat)
	case "default_view_mode":
		current = string(cfg.DefaultViewMode)
	}

	for i, it := range items {
		if it.(selectionItem).id == current {
			m.selectionList.Select(i)
			break
		}
	}
}

func (m *SettingsOverlayModel) applySetting(setting, value string) {
	cfg := config.Get()
	switch setting {
	case "theme":
		cfg.Theme = value
	case "icon_type":
		cfg.IconType = config.IconType(value)
	case "name_format":
		cfg.LibraryNameFormat = config.LibraryNameFormat(value)
	case "default_view_mode":
		cfg.DefaultViewMode = config.ViewMode(value)
	}
	_ = cfg.Save()
}

func (m *SettingsOverlayModel) View() string {
	overlayStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder(), true).
		BorderForeground(ui.Accent(m.theme)).
		Padding(1, 2).
		Background(lipgloss.Color("#111111"))

	var content string
	if m.isSelecting {
		content = m.selectionList.View()
	} else {
		content = m.list.View()
	}

	return overlayStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
		content,
		"\n [enter] change | [esc/q/s] back/close",
	))
}

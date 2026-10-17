package settings

import (
	"fmt"
	"strconv"

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
		settingItem{id: "terminal_video", title: "Terminal Video", description: "Video renderer for Ctrl+P", current: string(cfg.TerminalVideo)},
		settingItem{id: "poster_renderer", title: "Poster Renderer", description: "Native image protocol or half-cell text", current: cfg.PosterRenderer},
		settingItem{id: "terminal_video_mode", title: "Terminal Video Mode", description: "Suspend the TUI or overlay video while browsing", current: cfg.TerminalVideoMode},
		settingItem{id: "sixel_video_width", title: "Sixel Video Width", description: "Pixel cap for smoother playback; 0 uses the terminal size", current: strconv.Itoa(cfg.SixelVideoWidth)},
		settingItem{id: "window_music_visualizer", title: "Music Video Visualizer", description: "Music in an mpv window or terminal video", current: cfg.WindowMusicVisualizer},
		settingItem{id: "music_player", title: "Music Player", description: "Where music plays", current: cfg.MusicPlayer},
		settingItem{id: "music_visualizer", title: "Music Visualizer", description: "Audio-reactive terminal display (z to cycle)", current: cfg.MusicVisualizer},
	}
	m.list.SetItems(items)
}

func (m *SettingsOverlayModel) Init() tea.Cmd {
	return nil
}

func (m *SettingsOverlayModel) ActionRows() int { return 1 }

func (m *SettingsOverlayModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if mouse, ok := msg.(tea.MouseMsg); ok {
		mouse.X -= 3
		mouse.Y -= 2
		active := &m.list
		if m.isSelecting {
			active = &m.selectionList
		}
		previous := active.Index()
		clicked := ui.ListMouse(active, mouse)
		var preview tea.Cmd
		if m.isSelecting && m.activeSetting == "theme" && previous != active.Index() {
			selected := m.tints[active.Index()]
			m.theme = selected
			preview = func() tea.Msg { return ui.ThemeChangedMsg{Theme: selected} }
		}
		if clicked {
			model, cmd := m.Update(ui.KeyPress("enter"))
			return model, tea.Batch(cmd, preview)
		}
		return m, preview
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		listW := max(1, min(m.width-6, 88))
		listH := max(5, min(m.height-8, 30))
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

		if msg, ok := msg.(tea.KeyMsg); ok && msg.String() == "enter" && m.selectionList.SelectedItem() != nil {
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
	case "terminal_video":
		m.selectionList.Title = "Choose Terminal Video Renderer"
		items = []list.Item{
			selectionItem{id: string(config.VideoAuto), value: "Auto (detect terminal)"},
			selectionItem{id: string(config.VideoKitty), value: "Kitty graphics"},
			selectionItem{id: string(config.VideoSixel), value: "Sixel graphics"},
			selectionItem{id: string(config.VideoTCT), value: "TCT (text fallback)"},
		}
	case "poster_renderer":
		m.selectionList.Title = "Choose Poster Renderer"
		items = []list.Item{selectionItem{id: "auto", value: "Auto (Kitty, Sixel, half-cell)"},
			selectionItem{id: "kitty", value: "Kitty / Ghostty placeholders"},
			selectionItem{id: "sixel", value: "Sixel graphics"}, selectionItem{id: "halfcell", value: "Half-cell text"}}
	case "terminal_video_mode":
		m.selectionList.Title = "Choose Terminal Video Mode"
		items = []list.Item{selectionItem{id: "suspend", value: "Suspend TUI · full terminal and video controls"},
			selectionItem{id: "overlay", value: "Overlay · keep the TUI and player controls active"}}
	case "sixel_video_width":
		m.selectionList.Title = "Choose Sixel Pixel Width"
		for _, width := range []string{"320", "480", "640", "960", "1280", "0"} {
			label := width + " pixels"
			if width == "0" {
				label = "Automatic (full terminal)"
			}
			items = append(items, selectionItem{id: width, value: label})
		}
	case "window_music_visualizer":
		m.selectionList.Title = "Choose Music Video Visualizer"
		for _, style := range []string{"cqt", "spectrum", "waves", "scope", "off"} {
			items = append(items, selectionItem{id: style, value: style})
		}
	case "music_player":
		m.selectionList.Title = "Choose Music Player"
		items = []list.Item{
			selectionItem{id: "terminal", value: "In this terminal (audio + visualizer)"},
			selectionItem{id: "window", value: "Separate player window"},
		}
	case "music_visualizer":
		m.selectionList.Title = "Choose Music Visualizer"
		items = []list.Item{
			selectionItem{id: "waveform", value: "Waveform · live stereo waves and glowing trails (default)"},
			selectionItem{id: "spectrum", value: "Spectrum · gradient bars and falling peaks"},
			selectionItem{id: "mirror", value: "Mirror · symmetric spectrum"},
			selectionItem{id: "off", value: "Off · compact controls"},
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
	case "terminal_video":
		current = string(cfg.TerminalVideo)
	case "poster_renderer":
		current = cfg.PosterRenderer
	case "terminal_video_mode":
		current = cfg.TerminalVideoMode
	case "sixel_video_width":
		current = strconv.Itoa(cfg.SixelVideoWidth)
	case "window_music_visualizer":
		current = cfg.WindowMusicVisualizer
	case "music_player":
		current = cfg.MusicPlayer
	case "music_visualizer":
		current = cfg.MusicVisualizer
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
	case "terminal_video":
		cfg.TerminalVideo = config.VideoOutput(value)
	case "poster_renderer":
		cfg.PosterRenderer = value
	case "terminal_video_mode":
		cfg.TerminalVideoMode = value
	case "sixel_video_width":
		cfg.SixelVideoWidth, _ = strconv.Atoi(value)
	case "window_music_visualizer":
		cfg.WindowMusicVisualizer = value
	case "music_player":
		cfg.MusicPlayer = value
	case "music_visualizer":
		cfg.MusicVisualizer = value
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

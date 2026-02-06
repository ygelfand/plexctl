package tui

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/tui/player"
	"github.com/ygelfand/plexctl/internal/tui/view"
	"github.com/ygelfand/plexctl/internal/tui/view/detail"
	tuiconfig "github.com/ygelfand/plexctl/internal/tui/widget/config"
	"github.com/ygelfand/plexctl/internal/tui/widget/help"
	"github.com/ygelfand/plexctl/internal/tui/widget/resume"
	tuisearch "github.com/ygelfand/plexctl/internal/tui/widget/search"
	"github.com/ygelfand/plexctl/internal/tui/widget/settings"
	"github.com/ygelfand/plexctl/internal/ui"
	"go.dalton.dog/bubbleup"
)

type Controller struct {
	data  plex.LoaderResult
	theme tint.Tint

	tabManager *TabManager
	navigator  *Navigator
	player     *player.PlayerManager
	alert      bubbleup.AlertModel

	playerStatus player.PlayerStatus
	returnTabIdx int
}

func NewController(data plex.LoaderResult) *Controller {
	cfg := config.Get()
	tints := append([]tint.Tint{ui.PlexctlTheme}, tint.DefaultTints()...)
	activedTheme := tints[0]

	if cfg.Theme != "" {
		for _, t := range tints {
			if t.ID() == cfg.Theme {
				activedTheme = t
				break
			}
		}
	} else {
		cfg.Theme = activedTheme.ID()
		_ = cfg.Save()
	}

	ui.GetLayout().SetTheme(activedTheme)

	alert := bubbleup.NewAlertModel(40, true, 15*time.Second).
		WithPosition(bubbleup.TopRightPosition)
	alert.RegisterNewAlertType(bubbleup.AlertDefinition{
		Key:       "error",
		ForeColor: "#FF0000",
		Prefix:    "❌ ",
	})

	c := &Controller{
		data:         data,
		theme:        activedTheme,
		player:       player.GetPlayerManager(),
		navigator:    NewNavigator(activedTheme),
		alert:        alert,
		returnTabIdx: -1,
	}

	c.tabManager = NewTabManager(data, activedTheme, ui.SidebarWidth)

	return c
}

func (c *Controller) Init() tea.Cmd {
	var cmds []tea.Cmd
	if model := c.tabManager.ActiveModel(); model != nil {
		cmds = append(cmds, model.Init())
	}

	if overlay := c.navigator.ActiveOverlay(); overlay != nil {
		cmds = append(cmds, overlay.Init())
	}

	cmds = append(cmds, c.player.WaitForUpdates())
	cmds = append(cmds, c.player.PollUpdates())
	cmds = append(cmds, c.player.Reconnect())
	cmds = append(cmds, c.alert.Init())
	return tea.Batch(cmds...)
}

func (c *Controller) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	if err, ok := msg.(error); ok {
		slog.Error("TUI error", "error", err, "stack", string(debug.Stack()))
		cmds = append(cmds, c.alert.NewAlertCmd("error", err.Error()))
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		ui.GetLayout().Update(msg.Width, msg.Height, c.playerStatus.Active())
		for i, model := range c.tabManager.tabModels {
			if model != nil {
				c.tabManager.tabModels[i], _ = model.Update(msg)
			}
		}

	case settings.SettingsFinishedMsg:
		tints := append([]tint.Tint{ui.PlexctlTheme}, tint.DefaultTints()...)
		for _, t := range tints {
			if t.ID() == msg.Config.Theme {
				c.theme = t
				c.navigator.SetTheme(t)
				break
			}
		}
		return c, c.tabManager.setupTabs(c.data, c.theme, ui.SidebarWidth)

	case tuiconfig.ConfigFinishedMsg:
		cfg := config.Get()
		cfg.IconType = msg.IconType
		_ = cfg.Save()
		return c, c.tabManager.setupTabs(c.data, c.theme, ui.SidebarWidth)

	case ui.ThemeChangedMsg:
		ui.GetLayout().SetTheme(msg.Theme)
		c.theme = msg.Theme
		c.navigator.SetTheme(msg.Theme)
		c.tabManager.navbar.Update(msg)
		// Propagate theme to all tabs
		for i, model := range c.tabManager.tabModels {
			if model != nil {
				c.tabManager.tabModels[i], _ = model.Update(msg)
			}
		}
		return c, nil
	}

	if navCmd, captured := c.navigator.Update(msg); captured {
		return c, navCmd
	}

	switch msg := msg.(type) {
	case ui.RequestPlayMsg:
		return c, player.FetchAndPlay(msg.RatingKey, msg.TctMode)
	case ui.ResumeChoiceMsg:
		return c, c.navigator.Push(resume.NewResumeOverlayModel(msg.Metadata, msg.TctMode, c.theme))
	case ui.SelectMediaMsg:
		return c, c.handleSelectMedia(msg)
	case ui.JumpToDetailMsg:
		return c, c.handleJumpToDetail(msg)
	case player.PlayerStatusMsgPoller:
		if msg == (player.PlayerStatusMsgPoller{}) && !c.playerStatus.Active() {
			return c, nil
		}
		oldActive := c.playerStatus.Active()
		c.playerStatus = c.player.Status()
		if oldActive != c.playerStatus.Active() {
			ui.GetLayout().Update(ui.GetLayout().TotalWidth(), ui.GetLayout().TotalHeight(), c.playerStatus.Active())
			cmds = append(cmds, func() tea.Msg {
				return tea.WindowSizeMsg{Width: ui.GetLayout().TotalWidth(), Height: ui.GetLayout().TotalHeight()}
			})
		}
		if c.playerStatus.Active() {
			cmds = append(cmds, c.player.PollUpdates())
		}
		return c, tea.Batch(cmds...)
	case player.PlayerStatusMsg:
		oldActive := c.playerStatus.Active()
		c.playerStatus = c.player.Status()
		if oldActive != c.playerStatus.Active() {
			ui.GetLayout().Update(ui.GetLayout().TotalWidth(), ui.GetLayout().TotalHeight(), c.playerStatus.Active())
			cmds = append(cmds, func() tea.Msg {
				return tea.WindowSizeMsg{Width: ui.GetLayout().TotalWidth(), Height: ui.GetLayout().TotalHeight()}
			})
			if c.playerStatus.Active() {
				cmds = append(cmds, c.player.PollUpdates())
			}
		}
		return c, tea.Batch(append(cmds, c.player.WaitForUpdates())...)

	case tea.KeyMsg:
		slog.Log(context.Background(), config.LevelTrace, "TUI: key press", "key", msg.String())
		switch msg.String() {
		case "q", "ctrl+c":
			if config.Get().CloseVideoOnQuit {
				return c, c.player.Stop()
			}
			return c, tea.Quit
		case "s":
			return c, c.navigator.Push(settings.NewSettingsOverlayModel(c.theme))
		case "l":
			return c, c.navigator.Push(tuiconfig.NewConfigOverlayModel(c.data.Libraries))
		case "h":
			return c, c.tabManager.SetActive(0)
		case "p", "ctrl+p":
			tctMode := msg.String() == "ctrl+p"
			if provider, ok := c.tabManager.ActiveModel().(ui.PlayableProvider); ok {
				meta := provider.GetSelectedMetadata()
				if meta != nil && meta.RatingKey != nil {
					return c, func() tea.Msg {
						return ui.RequestPlayMsg{
							RatingKey: *meta.RatingKey,
							TctMode:   tctMode,
						}
					}
				}
			}
		case "?":
			return c, c.showHelp()
		case " ":
			if c.playerStatus.Active() {
				return c, c.player.TogglePause()
			}
		case "tab":
			return c, c.tabManager.NextTab()
		case "x":
			if c.playerStatus.Active() {
				return c, c.player.StopPlayback()
			}
		case "/":
			return c, c.navigator.Push(tuisearch.NewSearchOverlayModel(c.theme))
		case "shift+tab":
			return c, c.tabManager.PrevTab()
		}
	}

	var alertCmd tea.Cmd
	var alertModel tea.Model
	alertModel, alertCmd = c.alert.Update(msg)
	c.alert = alertModel.(bubbleup.AlertModel)
	cmds = append(cmds, alertCmd)

	if model := c.tabManager.ActiveModel(); model != nil {
		var cmd tea.Cmd
		c.tabManager.tabModels[c.tabManager.activeTabIdx], cmd = model.Update(msg)
		cmds = append(cmds, cmd)
	}

	if _, ok := msg.(detail.BackMsg); ok {
		if c.returnTabIdx != -1 {
			c.tabManager.SetActive(c.returnTabIdx)
			c.returnTabIdx = -1
		}
	}

	return c, tea.Batch(cmds...)
}

func (c *Controller) handleSelectMedia(msg ui.SelectMediaMsg) tea.Cmd {
	returnTabIdx := c.tabManager.activeTabIdx
	if msg.SectionID != "" {
		return func() tea.Msg {
			return ui.JumpToDetailMsg{
				SectionID:    msg.SectionID,
				RatingKey:    msg.RatingKey,
				Type:         msg.Type,
				ReturnTabIdx: returnTabIdx,
			}
		}
	}

	return func() tea.Msg {
		meta, err := plex.GetMetadata(context.Background(), msg.RatingKey, false)
		if err != nil {
			return err
		}
		sectionID := ""
		if sid, ok := meta.AdditionalProperties["librarySectionID"]; ok {
			if sidStr, ok := sid.(string); ok {
				sectionID = sidStr
			} else if sidFloat, ok := sid.(float64); ok {
				sectionID = fmt.Sprintf("%.0f", sidFloat)
			}
		}
		if sectionID != "" {
			return ui.JumpToDetailMsg{
				SectionID:    sectionID,
				RatingKey:    msg.RatingKey,
				Type:         msg.Type,
				ReturnTabIdx: returnTabIdx,
			}
		}
		return nil
	}
}

func (c *Controller) handleJumpToDetail(msg ui.JumpToDetailMsg) tea.Cmd {
	for i, tab := range c.tabManager.tabModels {
		if libTab, ok := tab.(*view.MediaView); ok {
			if libTab.GetSectionID() == msg.SectionID {
				c.tabManager.SetActive(i)
				c.returnTabIdx = msg.ReturnTabIdx
				return libTab.ShowDetail(msg.RatingKey, msg.Type)
			}
		}
	}
	return nil
}

func (c *Controller) showHelp() tea.Cmd {
	keys := []ui.HelpKey{
		{Key: "tab", Desc: "Switch Library"},
		{Key: "h", Desc: "Home"},
		{Key: "s", Desc: "Global Settings"},
		{Key: "l", Desc: "Libraries"},
		{Key: "p", Desc: "Play Selected"},
		{Key: "x", Desc: "Stop Playback"},
		{Key: "q", Desc: "Quit"},
		{Key: "?", Desc: "Help"},
	}

	if c.playerStatus.Active() {
		keys = append(keys, ui.HelpKey{Key: "space", Desc: "Play/Pause"})
	}

	if provider, ok := c.tabManager.ActiveModel().(ui.HelpProvider); ok {
		keys = append(keys, provider.HelpKeys()...)
	}

	return c.navigator.Push(help.NewHelpOverlayModel(keys, c.theme))
}

func (c *Controller) View() string {
	base := c.renderBaseView()
	base = c.navigator.Render(base)
	return c.alert.Render(base)
}

func (c *Controller) renderBaseView() string {
	layout := ui.GetLayout()
	if layout.TotalWidth() == 0 {
		return "Initializing..."
	}

	// Total height available for the main area (excluding footer)
	mainHeight := max(layout.TotalHeight()-1, 0)

	c.tabManager.navbar.Height = mainHeight
	sidebar := c.tabManager.navbar.View()

	// The content box should fill the remaining height
	// ContentHeight already accounts for player bar if active
	contentHeight := layout.ContentHeight()

	windowStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder(), true).
		BorderForeground(c.theme.BrightBlack()).
		Padding(0, 1).
		Width(layout.MainAreaContentWidth()).
		Height(contentHeight)

	content := ""
	if model := c.tabManager.ActiveModel(); model != nil {
		content = model.View()
	}

	body := windowStyle.Render(content)

	if c.playerStatus.Active() {
		body = lipgloss.JoinVertical(lipgloss.Left, body, c.renderPlayer())
	}

	// Join them at the top
	mainArea := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, body)

	footer := lipgloss.NewStyle().
		Width(layout.TotalWidth()).
		Background(c.theme.BrightBlack()).
		Foreground(c.theme.White()).
		Padding(0, 1).
		Render(" q: quit | tab: switch lib | h: home | s: settings | l: libs | p: play | x: stop ")

	return lipgloss.JoinVertical(lipgloss.Left, mainArea, footer)
}

func (c *Controller) renderPlayer() string {
	layout := ui.GetLayout()
	width := layout.MainAreaContentWidth()
	status := "▶ PLAYING"
	if c.playerStatus.Paused() {
		status = "⏸ PAUSED "
	}

	progress := 0.0
	if c.playerStatus.Duration > 0 {
		progress = c.playerStatus.Time / c.playerStatus.Duration
	}

	barWidth := max(width-35, 10)
	filled := int(float64(barWidth) * progress)
	empty := barWidth - filled

	accent := ui.Accent(c.theme)

	bar := lipgloss.NewStyle().Foreground(accent).Render(strings.Repeat("█", max(filled, 0))) +
		lipgloss.NewStyle().Foreground(c.theme.BrightBlack()).Render(strings.Repeat("░", max(empty, 0)))

	timeStr := fmt.Sprintf("%s / %s", formatDuration(c.playerStatus.Time), formatDuration(c.playerStatus.Duration))
	title := lipgloss.NewStyle().Foreground(c.theme.BrightYellow()).Bold(true).Width(width - 2).Render(c.playerStatus.Title)

	playerStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder(), true).BorderForeground(accent).Width(width).Padding(0, 1)

	row2 := lipgloss.JoinHorizontal(lipgloss.Center,
		lipgloss.NewStyle().Foreground(c.theme.BrightCyan()).Width(12).Render(status),
		bar,
		lipgloss.NewStyle().Width(20).Align(lipgloss.Right).Render(timeStr),
	)

	return playerStyle.Render(lipgloss.JoinVertical(lipgloss.Left, title, row2))
}

func formatDuration(seconds float64) string {
	h := int(seconds) / 3600
	m := (int(seconds) % 3600) / 60
	s := int(seconds) % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

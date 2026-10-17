package tui

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"
	"time"

	"github.com/LukeHagar/plexgo/models/operations"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/tui/artwork"
	"github.com/ygelfand/plexctl/internal/tui/player"
	"github.com/ygelfand/plexctl/internal/tui/view"
	"github.com/ygelfand/plexctl/internal/tui/view/detail"
	tuiconfig "github.com/ygelfand/plexctl/internal/tui/widget/config"
	"github.com/ygelfand/plexctl/internal/tui/widget/help"
	"github.com/ygelfand/plexctl/internal/tui/widget/letterjump"
	"github.com/ygelfand/plexctl/internal/tui/widget/playback"
	"github.com/ygelfand/plexctl/internal/tui/widget/poster"
	"github.com/ygelfand/plexctl/internal/tui/widget/resume"
	tuisearch "github.com/ygelfand/plexctl/internal/tui/widget/search"
	"github.com/ygelfand/plexctl/internal/tui/widget/settings"
	"github.com/ygelfand/plexctl/internal/tui/widget/userpicker"
	"github.com/ygelfand/plexctl/internal/tui/widget/visualizer"
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

	playerStatus    player.PlayerStatus
	returnTabIdx    int
	visualizer      visualizer.Model
	visualTick      time.Time
	focus           paneFocus
	focusedAlphabet *ui.AlphabetRail
	userSwitchID    uint64
	userPickerID    uint64
	reloadID        uint64
}

type usersLoadedMsg struct {
	id    uint64
	users []operations.HomeUser
	err   error
}

type switchUserSuccessMsg struct {
	id         uint64
	owner      Overlay
	homeUser   config.HomeUser
	invalidPin bool
	err        error
}
type reloadedDataMsg struct {
	id   uint64
	data plex.LoaderResult
	err  error
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

	cfg := config.Get()
	if cfg.HomeUser.AccessToken == "" || !cfg.AutoHomeLogin {
		cmds = append(cmds, c.triggerUserSwitch())
	}

	if model := c.tabManager.ActiveModel(); model != nil {
		cmds = append(cmds, model.Init())
	}

	if overlay := c.navigator.ActiveOverlay(); overlay != nil {
		cmds = append(cmds, overlay.Init())
	}

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
	case view.DataLoadedMsg:
		for _, model := range c.tabManager.tabModels {
			if msg.Owns(model) {
				_, cmd := model.Update(msg)
				return c, cmd
			}
		}
		return c, nil
	case view.SearchIndexProgressMsg, view.SearchIndexFinishedMsg:
		for _, model := range c.tabManager.tabModels {
			if status, ok := model.(*view.SearchStatusView); ok {
				_, cmd := status.Update(msg)
				cmds = append(cmds, cmd)
			}
		}
		return c, tea.Batch(cmds...)
	case view.FriendsServersMsg, view.FriendsLibrariesMsg:
		friends := c.tabManager.tabModels[c.tabManager.friendsIdx]
		_, cmd := friends.Update(msg)
		return c, cmd
	case poster.PosterLoadedMsg, detail.PosterDataMsg, detail.DataMsg:
		for _, model := range c.tabManager.tabModels {
			if model != nil {
				_, cmd := model.Update(msg)
				cmds = append(cmds, cmd)
			}
		}
		return c, tea.Batch(cmds...)
	case ui.MediaPageMsg:
		// Request IDs and server IDs keep asynchronous replies with their view.
		for _, model := range c.tabManager.tabModels {
			switch model := model.(type) {
			case *view.MediaView:
				_, cmd := model.Update(msg)
				cmds = append(cmds, cmd)
			case *view.FriendsView:
				_, cmd := model.Update(msg)
				cmds = append(cmds, cmd)
			}
		}
		return c, tea.Batch(cmds...)
	case tea.WindowSizeMsg:
		cmds = append(cmds, c.player.ResizeOverlay())
		ui.GetLayout().SetPlayerHeight(c.playerHeight(msg.Height))
		ui.GetLayout().Update(msg.Width, msg.Height, c.playerStatus.Active())
		for i, model := range c.tabManager.tabModels {
			if model != nil {
				var cmd tea.Cmd
				c.tabManager.tabModels[i], cmd = model.Update(msg)
				cmds = append(cmds, cmd)
			}
		}

	case settings.SettingsFinishedMsg:
		if err := artwork.Select(msg.Config.PosterRenderer); err != nil {
			return c, func() tea.Msg { return err }
		}
		tints := append([]tint.Tint{ui.PlexctlTheme}, tint.DefaultTints()...)
		for _, t := range tints {
			if t.ID() == msg.Config.Theme {
				c.theme = t
				c.navigator.SetTheme(t)
				break
			}
		}
		return c, tea.Batch(c.tabManager.setupTabs(c.data, c.theme, ui.SidebarWidth), resizePlayer(), tea.ClearScreen)

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

	case ui.UserSelectionMsg:
		return c, c.navigator.Push(userpicker.NewUserPickerOverlayModel(msg.Users, c.theme))
	case usersLoadedMsg:
		if msg.id != c.userPickerID {
			return c, nil
		}
		if msg.err != nil {
			return c, c.alert.NewAlertCmd("error", msg.err.Error())
		}
		if len(msg.users) <= 1 {
			return c, nil
		}
		return c.Update(ui.UserSelectionMsg{Users: msg.users})

	case ui.InvalidPinMsg:
		if overlay := c.navigator.ActiveOverlay(); overlay != nil {
			if picker, ok := overlay.(*userpicker.UserPickerOverlayModel); ok {
				var cmd tea.Cmd
				_, cmd = picker.Update(msg)
				return c, cmd
			}
		}
		return c, nil

	case ui.SwitchUserMsg:
		return c, c.handleSwitchUser(msg)

	case switchUserSuccessMsg:
		if msg.id != c.userSwitchID || msg.owner != c.navigator.ActiveOverlay() {
			return c, nil
		}
		if msg.invalidPin {
			return c.Update(ui.InvalidPinMsg{})
		}
		if msg.err != nil {
			return c, c.alert.NewAlertCmd("error", msg.err.Error())
		}
		cfg := config.Get()
		c.userPickerID++
		cfg.HomeUser = msg.homeUser
		if err := cfg.Save(); err != nil {
			cmds = append(cmds, c.alert.NewAlertCmd("error", err.Error()))
		}
		if msg.owner != nil {
			c.navigator.Pop()
		}
		// Retire views with the previous user's credentials before reloading.
		c.data = plex.LoaderResult{ServerID: cfg.DefaultServer}
		c.tabManager.activeTabIdx = 0
		c.returnTabIdx = -1
		cmds = append(cmds, c.tabManager.setupTabs(c.data, c.theme, ui.SidebarWidth), c.fullReload())
		return c, tea.Batch(cmds...)

	case reloadedDataMsg:
		if msg.id != c.reloadID {
			return c, nil
		}
		if msg.err != nil {
			return c, c.alert.NewAlertCmd("error", msg.err.Error())
		}
		c.data = msg.data
		return c, c.tabManager.setupTabs(c.data, c.theme, ui.SidebarWidth)
	}

	navCmd, captured := c.navigator.Update(msg)
	if captured {
		return c, navCmd
	}
	cmds = append(cmds, navCmd)
	if mouse, ok := msg.(tea.MouseMsg); ok {
		return c.handleMouse(mouse)
	}

	switch msg := msg.(type) {
	case ui.JumpToLetterMsg:
		if target, ok := c.tabManager.ActiveModel().(ui.LetterJumper); ok && target.CanJumpLetters() {
			c.focusPane(paneContent)
			return c, target.JumpToLetter(msg.Letter)
		}
		return c, nil
	case ui.RequestPlayMsg:
		return c, player.FetchAndPlay(msg.RatingKey, msg.Output, msg.Server)
	case ui.ResumeChoiceMsg:
		return c, c.navigator.Push(resume.NewResumeOverlayModel(msg.Metadata, msg.Output, c.theme, msg.Server))
	case ui.SelectMediaMsg:
		return c, c.handleSelectMedia(msg)
	case ui.JumpToDetailMsg:
		return c, c.handleJumpToDetail(msg)
	case player.PlayerStatusMsgPoller:
		cmds = append(cmds, c.refreshPlayer(), c.player.PollUpdates())
		return c, tea.Batch(cmds...)
	case player.PlayerStatusMsg:
		return c, c.refreshPlayer()
	case player.VideoFallbackMsg:
		return c, tea.Batch(c.refreshPlayer(), c.alert.NewAlertCmd("info", msg.String()))

	case tea.KeyMsg:
		slog.Log(context.Background(), config.LevelTrace, "TUI: key press", "key", msg.String())
		if cmd, handled := c.paneKey(msg.String()); handled {
			return c, cmd
		}
		switch msg.String() {
		case "g":
			if target, ok := c.tabManager.ActiveModel().(ui.LetterJumper); ok && target.CanJumpLetters() {
				return c, c.navigator.Push(letterjump.New(c.theme))
			}
		case "q", "ctrl+c":
			if config.Get().CloseVideoOnQuit || c.playerStatus.AudioOnly || c.playerStatus.Overlay {
				return c, c.player.Stop()
			}
			return c, tea.Quit
		case "ctrl+s":
			return c, c.navigator.Push(settings.NewSettingsOverlayModel(c.theme))
		case "L":
			return c, c.navigator.Push(tuiconfig.NewConfigOverlayModel(c.data.Libraries))
		case "H":
			c.focusPane(paneContent)
			return c, c.tabManager.SetActive(0)
		case "p", "ctrl+p", "v":
			output := config.VideoWindow
			if msg.String() == "ctrl+p" {
				output = config.Get().TerminalVideo
			}
			if provider, ok := c.tabManager.ActiveModel().(ui.PlayableProvider); ok {
				meta := provider.GetSelectedMetadata()
				if meta != nil && meta.RatingKey != nil {
					if config.IsMusic(meta.Type) {
						if msg.String() == "v" {
							return c, c.navigator.Push(playback.NewMusic(*meta.RatingKey, meta.Title, c.theme, c.mediaServer()))
						}
						output = config.Get().MusicOutput()
						if msg.String() == "ctrl+p" {
							output = config.AudioTerminal
						}
					}
					if msg.String() == "v" {
						return c, c.navigator.Push(playback.New(*meta.RatingKey, meta.Title, c.theme, c.mediaServer()))
					}
					server := c.mediaServer()
					return c, func() tea.Msg {
						return ui.RequestPlayMsg{Server: server,
							RatingKey: *meta.RatingKey,
							Output:    output,
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
			c.focusPane(paneContent)
			return c, c.tabManager.NextTab()
		case "x":
			if c.playerStatus.Active() {
				return c, c.player.StopPlayback()
			}
		case "[", "]", "+", "=", "-", "m", "z":
			if c.playerStatus.Active() {
				return c, c.musicControl(msg.String())
			}
		case "/":
			server := c.mediaServer()
			if server.ID == "" {
				return c, c.alert.NewAlertCmd("error", "Choose a friend's server to search its shared libraries")
			}
			return c, c.navigator.Push(tuisearch.NewSearchOverlayModel(c.theme, server))
		case "u":
			return c, c.triggerUserSwitch()
		case "shift+tab":
			c.focusPane(paneContent)
			return c, c.tabManager.PrevTab()
		}
		if c.focus != paneContent {
			return c, nil
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
	server := config.MediaServer(msg.Server)
	if !server.Shared && server.CacheNamespace() != config.Get().MediaCacheNamespace() {
		return nil
	}
	returnTabIdx := c.tabManager.activeTabIdx
	if msg.SectionID != "" || server.Shared {
		return func() tea.Msg {
			return ui.JumpToDetailMsg{Server: server,
				SectionID:    msg.SectionID,
				RatingKey:    msg.RatingKey,
				Type:         msg.Type,
				ReturnTabIdx: returnTabIdx,
			}
		}
	}

	return func() tea.Msg {
		meta, err := plex.GetMetadata(server.Context(context.Background()), msg.RatingKey, false)
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
			return ui.JumpToDetailMsg{Server: server,
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
	if msg.Server.ID != "" && !msg.Server.Shared && msg.Server.CacheNamespace() != config.Get().MediaCacheNamespace() {
		return nil
	}
	if msg.Server.Shared {
		friends := c.tabManager.tabModels[c.tabManager.friendsIdx].(*view.FriendsView)
		c.returnTabIdx = -1
		c.focusPane(paneContent)
		c.tabManager.SetActive(c.tabManager.friendsIdx)
		return friends.ShowDetail(msg)
	}
	slog.Debug("Controller: handleJumpToDetail", "key", msg.RatingKey, "type", msg.Type, "section", msg.SectionID)

	// 1. If SectionID is specified, find that specific tab
	if msg.SectionID != "" {
		for i, tab := range c.tabManager.tabModels {
			if libTab, ok := tab.(*view.MediaView); ok {
				if libTab.GetSectionID() == msg.SectionID {
					slog.Debug("Controller: Found matching library tab", "idx", i)
					c.focusPane(paneContent)
					initCmd := c.tabManager.SetActive(i)
					c.returnTabIdx = msg.ReturnTabIdx
					return tea.Batch(initCmd, libTab.ShowDetail(msg.RatingKey, msg.Type))
				}
			}
		}
	}

	// 2. Fallback: If no section specified OR not found, try the active tab
	if activeTab, ok := c.tabManager.ActiveModel().(*view.MediaView); ok {
		slog.Debug("Controller: Applying jump to active library tab")
		c.focusPane(paneContent)
		return activeTab.ShowDetail(msg.RatingKey, msg.Type)
	}

	slog.Warn("Controller: No matching library tab found for jump", "section", msg.SectionID)
	return nil
}

func (c *Controller) handleSwitchUser(msg ui.SwitchUserMsg) tea.Cmd {
	c.userSwitchID++
	id, owner := c.userSwitchID, c.navigator.ActiveOverlay()
	serverID, _, _ := config.Get().GetActiveServer()
	client, clientErr := plex.NewHomeHomeUserClient()
	return func() tea.Msg {
		result := switchUserSuccessMsg{id: id, owner: owner}
		if clientErr != nil {
			result.err = clientErr
			return result
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		slog.Info("TUI: Switching user", "target", msg.User.Title, "uuid", msg.User.UUID)

		req := operations.SwitchUserRequest{
			ID: msg.User.UUID,
		}
		if msg.Pin != "" {
			req.RequestBody = operations.SwitchUserRequestBody{
				Pin: &msg.Pin,
			}
		}

		res, err := client.SDK.HomeUsers.SwitchUser(ctx, req)
		if err != nil {
			// Check for 403/Forbidden which indicates invalid PIN
			if strings.Contains(err.Error(), "403") || strings.Contains(strings.ToLower(err.Error()), "pin is required") {
				result.invalidPin = true
				return result
			}
			slog.Error("TUI: Switch user failed", "error", err)
			result.err = err
			return result
		}

		if res.UserPlexAccount == nil || res.UserPlexAccount.AuthToken == "" {
			result.err = fmt.Errorf("failed to switch user: no account data returned")
			return result
		}

		// 2. We got an AuthToken. Now we need to use THIS specific AuthToken to get resources
		slog.Info("TUI: Fetching server-specific token for home user")
		newClient, err := plex.NewAccountClientWithToken(res.UserPlexAccount.AuthToken)
		if err != nil {
			result.err = err
			return result
		}

		resources, err := newClient.SDK.Plex.GetServerResources(ctx, operations.GetServerResourcesRequest{
			IncludeHTTPS: operations.IncludeHTTPSTrue.ToPointer(),
		})
		if err != nil {
			slog.Error("TUI: Failed to get server resources for home user", "error", err)
			result.err = err
			return result
		}

		accessToken := ""
		for _, dev := range resources.PlexDevices {
			if dev.ClientIdentifier == serverID && dev.AccessToken != "" {
				slog.Info("TUI: Found server-specific token", "server", dev.Name)
				accessToken = dev.AccessToken
				break
			}
		}
		if accessToken == "" {
			result.err = fmt.Errorf("selected user does not have access to this server")
			return result
		}

		result.homeUser = config.HomeUser{AuthToken: res.UserPlexAccount.AuthToken, AccessToken: accessToken, ServerID: serverID}
		return result
	}
}

func (c *Controller) triggerUserSwitch() tea.Cmd {
	c.userPickerID++
	id := c.userPickerID
	client, clientErr := plex.NewHomeHomeUserClient()
	return func() tea.Msg {
		if clientErr != nil {
			return usersLoadedMsg{id: id, err: clientErr}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		res, err := client.SDK.HomeUsers.GetHomeUsers(ctx)
		if err != nil {
			return usersLoadedMsg{id: id, err: err}
		}
		if res.Object == nil {
			return usersLoadedMsg{id: id, err: fmt.Errorf("no home data returned")}
		}
		return usersLoadedMsg{id: id, users: res.Object.Users}
	}
}

func (c *Controller) fullReload() tea.Cmd {
	c.reloadID++
	id, server := c.reloadID, config.Get().ServerTarget()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(server.Context(context.Background()), 2*time.Minute)
		defer cancel()
		updates := make(chan any, 10)
		go plex.LoadData(ctx, updates)

		for {
			var msg any
			select {
			case msg = <-updates:
			case <-ctx.Done():
				return reloadedDataMsg{id: id, err: ctx.Err()}
			}
			if res, ok := msg.(plex.LoaderResult); ok {
				return reloadedDataMsg{id: id, data: res}
			}
			if err, ok := msg.(error); ok {
				return reloadedDataMsg{id: id, err: err}
			}
		}
	}
}

func (c *Controller) showHelp() tea.Cmd {
	keys := []ui.HelpKey{
		{Key: "h / j / k / l", Desc: "Navigate content: left / down / up / right (or arrow keys)"},
		{Key: "click", Desc: "Open posters, rows, navigation, and actions"},
		{Key: "wheel", Desc: "Scroll under pointer; Shift+wheel browses a poster row"},
		{Key: "right click", Desc: "Back / close dialog (Shift bypasses mouse capture)"},
		{Key: "tab", Desc: "Switch Library"},
		{Key: "ctrl+h / j / k / l", Desc: "Focus pane: left / down / up / right"},
		{Key: "j / k, enter", Desc: "In navigation: select a library, then open it"},
		{Key: "ctrl+l, j / k, enter", Desc: "From content: focus A–Z, choose a letter, then jump"},
		{Key: "g, then A–Z / #", Desc: "Jump to a letter (or click the right-hand alphabet)"},
		{Key: "Home / End / PgUp / PgDn", Desc: "First / last item / previous / next page"},
		{Key: "H", Desc: "Home"},
		{Key: "ctrl+s", Desc: "Global Settings"},
		{Key: "L", Desc: "Libraries"},
		{Key: "u", Desc: "Switch User"},
		{Key: "p", Desc: "Play Selected"},
		{Key: "ctrl+p", Desc: "Play in Terminal"},
		{Key: "v", Desc: "Choose player (music: terminal / window; video: renderer)"},
		{Key: "[ / ]", Desc: "Seek backward / forward 10 seconds"},
		{Key: "+ / - / m", Desc: "Volume up / down / mute"},
		{Key: "z", Desc: "Cycle music visualizer: waveform / spectrum / mirror / off"},
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
	c.player.SetOverlayVisible(c.navigator.ActiveOverlay() == nil)
	base := c.renderBaseView()
	base = c.navigator.Render(base)
	return artwork.Frame(c.alert.Render(base))
}

func (c *Controller) renderBaseView() string {
	layout := ui.GetLayout()
	if layout.TotalWidth() == 0 {
		return "Initializing..."
	}

	// Total height available for the main area (excluding footer)
	mainHeight := max(layout.TotalHeight()-1, 0)

	c.tabManager.navbar.Height = mainHeight
	c.tabManager.navbar.SetFocused(c.focus == paneSidebar)
	sidebar := c.tabManager.navbar.View()

	// The content box should fill the remaining height
	// ContentHeight already accounts for player bar if active
	contentHeight := layout.ContentHeight()

	content := ""
	if model := c.tabManager.ActiveModel(); model != nil {
		content = model.View()
	}
	c.validateAlphabetFocus()

	windowStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder(), true).
		BorderForeground(c.paneBorder(paneContent)).
		Padding(0, 1).
		Width(layout.MainAreaContentWidth() + 2).
		Height(contentHeight)

	content = lipgloss.NewStyle().MaxWidth(layout.InnerWidth()).MaxHeight(contentHeight).Render(content)
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
		Render(c.footerText())

	return lipgloss.JoinVertical(lipgloss.Left, mainArea, footer)
}

func (c *Controller) footerText() string {
	text := "Ctrl+HJKL: panes | [p] Play | [v] Player | [V] View | [/] Search | [?] Help | [q] Quit"
	if c.playerStatus.Active() {
		text = "Ctrl+HJKL: panes | [p] Play | [v] Player | [x] Stop | [/] Search | [?] Help | [q] Quit"
	}
	switch c.focus {
	case paneSidebar:
		text = "Navigation | j/k: select | [enter] Open | Ctrl+L: content | [L] Libraries | [?] Help"
	case panePlayer:
		text = "Player | h/l: seek | j/k: volume | [space] Pause | Ctrl+K: content | [?] Help"
	case paneAlphabet:
		text = "Alphabet | j/k or ↑/↓: select | [enter] Jump | Ctrl+H: content | [esc] Cancel | [?] Help"
	}
	return ui.Ellipsis(text, max(1, ui.GetLayout().TotalWidth()-2))
}

func (c *Controller) handleMouse(mouse tea.MouseMsg) (tea.Model, tea.Cmd) {
	layout := ui.GetLayout()
	if ui.RightClick(mouse) {
		c.focusPane(paneContent)
		return c.Update(ui.KeyPress("esc"))
	}
	if mouse.X < ui.SidebarWidth && mouse.Y < layout.TotalHeight()-1 {
		if delta := ui.WheelDelta(mouse); delta != 0 {
			c.tabManager.navbar.Scroll(delta * 3)
		} else if ui.LeftClick(mouse) {
			c.focusPane(paneSidebar)
			if index := c.tabManager.navbar.ItemAt(mouse); index >= 0 {
				return c, c.tabManager.SetActive(index)
			}
			if key := c.tabManager.navbar.ActionAt(mouse); key != "" {
				return c.Update(ui.KeyPress(key))
			}
		}
		return c, nil
	}
	if mouse.Y == layout.TotalHeight()-1 {
		local := mouse
		local.X-- // Footer padding.
		local.Y = 0
		if key := ui.ShortcutAt(c.footerText(), local); key != "" {
			return c.Update(ui.KeyPress(key))
		}
		return c, nil
	}
	rect := ui.Rect{X: ui.SidebarTotalWidth + 2, Y: 1, Width: layout.InnerWidth(), Height: layout.ContentHeight()}
	local, inside := rect.Local(mouse)
	if !inside {
		if c.playerStatus.Active() && mouse.X > ui.SidebarTotalWidth &&
			mouse.Y >= rect.Height+2 && mouse.Y < layout.TotalHeight()-1 {
			if ui.LeftClick(mouse) {
				c.focusPane(panePlayer)
			}
			return c, c.playerMouse(mouse, rect.Height+2)
		}
		return c, nil
	}
	if ui.LeftClick(mouse) {
		c.focusPane(paneContent)
	}
	if model := c.tabManager.ActiveModel(); model != nil {
		// Detail action bars appear at the bottom of their own view.
		view := model.View()
		if actions, ok := model.(interface{ ActionRows() int }); ok {
			if local.Y >= lipgloss.Height(view)-actions.ActionRows() {
				if key := ui.ShortcutAt(view, local); key != "" {
					return c.Update(ui.KeyPress(key))
				}
			}
		}
		var cmd tea.Cmd
		c.tabManager.tabModels[c.tabManager.activeTabIdx], cmd = model.Update(local)
		return c, cmd
	}
	return c, nil
}

func (c *Controller) renderPlayer() string {
	layout := ui.GetLayout()
	if c.playerStatus.AudioOnly {
		return c.renderMusicPlayer()
	}
	width := layout.MainAreaContentWidth()
	status := "▶ PLAYING"
	if c.playerStatus.Paused() {
		status = "⏸ PAUSED "
	}

	progress := 0.0
	if c.playerStatus.Duration > 0 {
		progress = max(0, min(1, c.playerStatus.Time/c.playerStatus.Duration))
	}

	barWidth := max(width-35, 10)
	filled := int(float64(barWidth) * progress)
	empty := barWidth - filled

	accent := ui.Accent(c.theme)

	bar := lipgloss.NewStyle().Foreground(accent).Render(strings.Repeat("█", max(filled, 0))) +
		lipgloss.NewStyle().Foreground(c.theme.BrightBlack()).Render(strings.Repeat("░", max(empty, 0)))

	timeStr := fmt.Sprintf("%s / %s", ui.FormatDuration(int(c.playerStatus.Time*1000)), ui.FormatDuration(int(c.playerStatus.Duration*1000)))
	title := lipgloss.NewStyle().Foreground(c.theme.BrightYellow()).Bold(true).Width(width - 2).Render(c.playerStatus.Title)

	playerStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder(), true).
		BorderForeground(c.paneBorder(panePlayer)).Width(width).Padding(0, 1)

	row2 := lipgloss.JoinHorizontal(lipgloss.Center,
		lipgloss.NewStyle().Foreground(c.theme.BrightCyan()).Width(12).Render(status),
		bar,
		lipgloss.NewStyle().Width(20).Align(lipgloss.Right).Render(timeStr),
	)

	return playerStyle.Render(lipgloss.JoinVertical(lipgloss.Left, title, row2))
}

func (c *Controller) mediaServer() config.ServerTarget {
	if provider, ok := c.tabManager.ActiveModel().(ui.ServerProvider); ok {
		return provider.MediaServer()
	}
	return config.Get().ServerTarget()
}

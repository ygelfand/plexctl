package view

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/cache"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/presenters"
	"github.com/ygelfand/plexctl/internal/tui/view/detail"
	"github.com/ygelfand/plexctl/internal/tui/widget/poster"
	"github.com/ygelfand/plexctl/internal/ui"
)

type ViewMode int

const (
	ViewModeList ViewMode = iota
	ViewModePoster
)

type MediaView struct {
	server      config.ServerTarget
	sectionID   string
	title       string
	table       ui.Table
	posterGrid  *poster.PosterGrid
	theme       tint.Tint
	err         error
	totalItems  int
	loadedItems int
	isLoading   bool
	hasMore     bool
	requestID   uint64
	allMetadata []components.Metadata
	viewMode    ViewMode
	rail        ui.AlphabetRail
	contentRect ui.Rect
	jumpLetter  string
	jumpEnd     bool
	jumpNote    string

	details detail.DetailManager
}

var pageRequestID atomic.Uint64

func NewMediaView(sectionID, title string, theme tint.Tint, servers ...config.ServerTarget) *MediaView {
	serverTarget := config.MediaServer(servers...)
	columns := []table.Column{
		{Title: "W", Width: 3},
		{Title: "TITLE", Width: 45},
		{Title: "TYPE", Width: 10},
		{Title: "YEAR", Width: 6},
		{Title: "DURATION", Width: 10},
		{Title: "R", Width: 5},
		{Title: "CONTENT", Width: 10},
		{Title: "GENRE", Width: 15},
	}

	t := ui.NewTable(columns, theme)

	cfg := config.Get()
	mode := config.ViewModeList
	if cfg.DefaultViewMode != "" {
		mode = cfg.DefaultViewMode
	}

	_, server, ok := cfg.GetActiveServer()
	if ok && serverTarget.ID == cfg.DefaultServer {
		if opts, ok := server.Libraries.Settings[sectionID]; ok && opts.ViewMode != "" {
			mode = opts.ViewMode
		}
	}

	vMode := ViewModeList
	if mode == config.ViewModePoster {
		vMode = ViewModePoster
	}

	return &MediaView{
		server:      serverTarget,
		sectionID:   sectionID,
		title:       title,
		table:       t,
		posterGrid:  poster.NewPosterGrid(nil, theme, serverTarget),
		theme:       theme,
		viewMode:    vMode,
		allMetadata: []components.Metadata{},
		totalItems:  -1,
		hasMore:     true,
	}
}

func (v *MediaView) Init() tea.Cmd {
	if v.loadedItems > 0 || v.isLoading {
		return nil
	}
	v.isLoading = true
	return v.fetchPage(0)
}

func (v *MediaView) Refresh() tea.Cmd {
	if v.details.Active() {
		if refresher, ok := v.details.View.(ui.Refreshable); ok {
			return refresher.Refresh()
		}
	}
	v.allMetadata = nil
	v.loadedItems = 0
	v.err = nil
	v.jumpLetter, v.jumpNote, v.jumpEnd = "", "", false
	v.totalItems = -1
	v.hasMore = true
	v.table.SetRows(nil)
	v.posterGrid.SetItems(nil)
	v.posterGrid.SetCursor(0)
	v.table.SetCursor(0)
	v.isLoading = true
	return v.fetchPage(0)
}

func (v *MediaView) fetchPage(start int) tea.Cmd {
	sectionID, requestID := v.sectionID, pageRequestID.Add(1)
	v.requestID = requestID
	return func() tea.Msg {
		page := ui.MediaPageMsg{ServerID: v.server.ID, SectionID: sectionID, RequestID: requestID, Start: start, Total: -1}
		ctx, cancel := context.WithTimeout(v.server.Context(context.Background()), 30*time.Second)
		defer cancel()
		slog.Debug("MediaView: fetchPage started", "section", sectionID, "start", start)
		client, err := plex.NewClientForContext(ctx)
		if err != nil {
			page.Err = err
			return page
		}

		cfg := config.Get()
		cm, _ := cache.Get(cfg.CacheDir)

		req := operations.ListContentRequest{
			SectionID:           sectionID,
			XPlexContainerStart: ui.Ptr(start),
			XPlexContainerSize:  ui.Ptr(100),
			MediaQuery:          &components.MediaQuery{Sort: ui.Ptr("titleSort:asc")},
		}

		var body components.MediaContainerWithMetadata
		err = cache.AutoCache(cm, v.server.CacheNamespace(), req, plex.LibraryCacheTTL, &body, func() (*components.MediaContainerWithMetadata, error) {
			res, err := client.SDK.Content.ListContent(ctx, req)
			if err != nil {
				return nil, err
			}
			if res.MediaContainerWithMetadata == nil {
				return nil, fmt.Errorf("no metadata")
			}
			return res.MediaContainerWithMetadata, nil
		})
		if err != nil {
			page.Err = err
			return page
		}

		if body.MediaContainer == nil {
			page.Err = fmt.Errorf("no media container found")
			return page
		}
		if body.MediaContainer.TotalSize != nil {
			page.Total = int(*body.MediaContainer.TotalSize)
		}
		page.Metadata = body.MediaContainer.Metadata
		return page
	}
}

func (v *MediaView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(detail.DataMsg); ok && !v.details.Active() {
		return v, nil
	}
	if posterMsg, ok := msg.(poster.PosterLoadedMsg); ok {
		v.posterGrid.Update(posterMsg)
	}
	if v.details.Active() {
		var parentCmd tea.Cmd
		// Parent-level updates still happen even if detail is active (like window size)
		switch m := msg.(type) {
		case tea.WindowSizeMsg:
			v.updateBounds()
			parentCmd = v.posterGrid.Init()
		case ui.ThemeChangedMsg:
			v.theme = m.Theme
			v.posterGrid.Theme = v.theme
			ui.UpdateTableTheme(&v.table, v.contentRect.Width, ui.GetLayout().TotalHeight())
			v.table.SetHeight(v.contentRect.Height)
		}

		cmd, handled := v.details.Update(msg)
		if handled {
			return v, tea.Batch(parentCmd, cmd)
		}
	}

	var cmds []tea.Cmd
	if mouse, ok := msg.(tea.MouseMsg); ok {
		if letter := v.rail.LetterAt(mouse); letter != "" {
			return v, v.JumpToLetter(letter)
		}
		if _, inside := v.contentRect.Local(mouse); !inside {
			return v, nil
		}
		if v.viewMode == ViewModeList && v.table.Click(mouse) {
			msg = ui.KeyPress("enter")
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.updateBounds()
		cmds = append(cmds, v.posterGrid.Init())
		return v, tea.Batch(cmds...)
	case ui.ThemeChangedMsg:
		v.theme = msg.Theme
		v.posterGrid.Theme = v.theme
		ui.UpdateTableTheme(&v.table, v.contentRect.Width, ui.GetLayout().TotalHeight())
		v.table.SetHeight(v.contentRect.Height)
	case ui.MediaPageMsg:
		if (msg.ServerID != "" && msg.ServerID != v.server.ID) || msg.SectionID != v.sectionID || msg.RequestID != v.requestID || msg.Start != v.loadedItems {
			return v, nil
		}
		v.isLoading = false
		v.err = msg.Err
		if msg.Err != nil {
			return v, nil
		}
		v.totalItems = msg.Total
		v.allMetadata = append(v.allMetadata, msg.Metadata...)
		v.loadedItems = len(v.allMetadata)
		v.hasMore = len(msg.Metadata) > 0 && (msg.Total < 0 || v.loadedItems < msg.Total)
		v.syncTableRows()
		cmds = append(cmds, v.posterGrid.SetItems(v.allMetadata))
		if v.jumpLetter != "" || v.jumpEnd {
			cmds = append(cmds, v.advanceJump())
		} else {
			cmds = append(cmds, v.loadMore())
		}
		return v, tea.Batch(cmds...)
	case poster.PosterLoadedMsg:
		_, cmd := v.posterGrid.Update(msg)
		return v, cmd
	case poster.ItemSelectedMsg:
		if msg.Metadata.RatingKey != nil {
			return v, v.ShowDetail(*msg.Metadata.RatingKey, msg.Metadata.Type)
		}
	case error:
		v.err = msg
	case tea.KeyMsg:
		switch msg.String() {
		case "home":
			v.jumpLetter, v.jumpNote, v.jumpEnd = "", "", false
			return v, v.selectIndex(0)
		case "end":
			v.jumpLetter, v.jumpNote, v.jumpEnd = "", "", true
			return v, v.advanceJump()
		case "esc":
			v.jumpLetter, v.jumpNote, v.jumpEnd = "", "", false
			return v, nil
		case "V":
			cfg := config.Get()
			id, server, ok := cfg.GetActiveServer()

			if v.viewMode == ViewModeList {
				v.viewMode = ViewModePoster
				cmds = append(cmds, v.posterGrid.SetCursor(v.table.Cursor()))
			} else {
				v.viewMode = ViewModeList
				v.table.SetCursor(v.posterGrid.Cursor)
			}

			if ok && id == v.server.ID {
				if server.Libraries.Settings == nil {
					server.Libraries.Settings = make(map[string]config.LibraryOptions)
				}
				opts := server.Libraries.Settings[v.sectionID]
				opts.ViewMode = config.ViewModeList
				if v.viewMode == ViewModePoster {
					opts.ViewMode = config.ViewModePoster
				}
				server.Libraries.Settings[v.sectionID] = opts
				cfg.Servers[id] = server
				_ = cfg.Save()
			}
			return v, tea.Batch(cmds...)
		case "enter":
			currIdx := v.table.Cursor()
			if v.viewMode == ViewModePoster {
				currIdx = v.posterGrid.Cursor
			}

			if currIdx >= 0 && currIdx < len(v.allMetadata) {
				meta := v.allMetadata[currIdx]
				ratingKey := ""
				if meta.RatingKey != nil {
					ratingKey = *meta.RatingKey
				}
				return v, v.ShowDetail(ratingKey, meta.Type)
			}
		case "r":
			if v.err != nil {
				v.err = nil
				v.isLoading = true
				return v, v.fetchPage(v.loadedItems)
			}
			return v, v.Refresh()
		}
	}

	var cmd tea.Cmd
	if v.viewMode == ViewModeList {
		v.table, cmd = v.table.Update(msg)
	} else {
		_, cmd = v.posterGrid.Update(msg)
	}
	cmds = append(cmds, cmd)

	cmds = append(cmds, v.loadMore())

	return v, tea.Batch(cmds...)
}

func (v *MediaView) loadMore() tea.Cmd {
	if v.isLoading || v.err != nil || !v.hasMore || v.details.Active() {
		return nil
	}
	index := v.table.Cursor()
	if v.viewMode == ViewModePoster {
		index = max(v.posterGrid.Cursor, v.posterGrid.LastVisibleIndex())
	}
	if index >= v.loadedItems-20 {
		v.isLoading = true
		return v.fetchPage(v.loadedItems)
	}
	return nil
}

func (v *MediaView) CanJumpLetters() bool {
	if v.details.Active() {
		target, ok := v.details.View.(ui.LetterJumper)
		return ok && target.CanJumpLetters()
	}
	return true
}

func (v *MediaView) AlphabetRail() *ui.AlphabetRail {
	if v.details.Active() {
		if target, ok := v.details.View.(ui.AlphabetNavigator); ok {
			return target.AlphabetRail()
		}
		return nil
	}
	if v.err != nil && v.loadedItems == 0 {
		return nil
	}
	return &v.rail
}

func (v *MediaView) JumpToLetter(letter string) tea.Cmd {
	if v.details.Active() {
		if target, ok := v.details.View.(ui.LetterJumper); ok {
			return target.JumpToLetter(letter)
		}
		return nil
	}
	letter = strings.ToUpper(letter)
	if len(letter) != 1 || !strings.Contains(ui.AlphabetLetters, letter) {
		return nil
	}
	v.jumpLetter, v.jumpNote, v.jumpEnd = letter, "", false
	return v.advanceJump()
}

func (v *MediaView) selectIndex(index int) tea.Cmd {
	v.table.SetCursor(index)
	return v.posterGrid.SetCursor(index)
}

// A jump reuses already fetched pages, then loads missing metadata as needed.
// Only visible artwork is fetched; repeated jumps are immediate once loaded.
func (v *MediaView) advanceJump() tea.Cmd {
	if v.jumpLetter != "" {
		if i := ui.AlphabetIndex(v.allMetadata, v.jumpLetter); i >= 0 {
			v.jumpLetter = ""
			return v.selectIndex(i)
		}
	}
	if !v.hasMore && !v.isLoading {
		if v.jumpEnd {
			v.jumpEnd = false
			return v.selectIndex(max(0, v.loadedItems-1))
		}
		v.jumpNote = "No titles under " + v.jumpLetter
		v.jumpLetter = ""
		return nil
	}
	if !v.isLoading && v.err == nil {
		v.isLoading = true
		return v.fetchPage(v.loadedItems)
	}
	return nil
}

func (v *MediaView) updateBounds() {
	layout := ui.GetLayout()
	height := max(1, layout.ContentHeight()-1)
	v.rail.SetBounds(layout.InnerWidth(), 0, height)
	rect := ui.Rect{Width: max(1, layout.InnerWidth()-v.rail.Width), Height: height}
	if rect != v.contentRect {
		v.contentRect = rect
		v.table.SetHeight(rect.Height)
		v.table.SetWidth(rect.Width)
		v.posterGrid.SetSize(rect.Width, rect.Height)
	}
}

func (v *MediaView) GetSectionID() string {
	return v.sectionID
}

func (v *MediaView) GetSelectedMetadata() *components.Metadata {
	if v.details.Active() {
		if provider, ok := v.details.View.(ui.PlayableProvider); ok {
			return provider.GetSelectedMetadata()
		}
		return nil
	}

	currIdx := v.table.Cursor()
	if v.viewMode == ViewModePoster {
		currIdx = v.posterGrid.Cursor
	}

	if currIdx >= 0 && currIdx < len(v.allMetadata) {
		return &v.allMetadata[currIdx]
	}
	return nil
}

func (v *MediaView) ShowDetail(ratingKey, mediaType string) tea.Cmd {
	var dv tea.Model
	switch mediaType {
	case "show":
		dv = detail.NewShowDetailView(ratingKey, v.theme, v.server)
	case "season":
		dv = detail.NewSeasonDetailView(ratingKey, v.theme, v.server)
	case "episode":
		dv = detail.NewEpisodeDetailView(ratingKey, v.theme, v.server)
	case "artist", "album", "track":
		dv = detail.NewMusicDetailView(ratingKey, v.theme, v.server)
	default:
		dv = detail.NewMovieDetailView(ratingKey, v.theme, v.server)
	}

	layout := ui.GetLayout()
	if sizer, ok := dv.(interface{ SetSize(int, int) }); ok {
		sizer.SetSize(layout.InnerWidth(), layout.ContentHeight())
	}

	return v.details.Set(dv)
}

func (v *MediaView) HelpKeys() []ui.HelpKey {
	if provider, ok := v.details.View.(ui.HelpProvider); ok {
		return provider.HelpKeys()
	}
	return []ui.HelpKey{
		{Key: "enter", Desc: "View Details"},
		{Key: "V", Desc: "Toggle List / Poster View"},
		{Key: "k/up", Desc: "Move Up"},
		{Key: "j/down", Desc: "Move Down"},
	}
}

func (v *MediaView) View() string {
	if v.details.Active() {
		return v.details.ViewContent()
	}
	if v.err != nil && v.loadedItems == 0 {
		return fmt.Sprintf("Unable to load this page: %v\nPress r to retry.", v.err)
	}
	v.updateBounds()
	content := ""
	if v.viewMode == ViewModeList {
		content = v.table.View()
	} else {
		content = v.posterGrid.View()
	}
	active := v.jumpLetter
	if active == "" {
		if meta := v.GetSelectedMetadata(); meta != nil {
			active = ui.AlphabetLetter(*meta)
		}
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(v.contentRect.Width).Height(v.contentRect.Height).
			MaxWidth(v.contentRect.Width).MaxHeight(v.contentRect.Height).Render(content),
		v.rail.View(active, v.theme))
	count := fmt.Sprintf("%d loaded", v.loadedItems)
	if v.totalItems >= 0 {
		count = fmt.Sprintf("%d / %d", v.loadedItems, v.totalItems)
	}
	status := v.jumpNote
	switch {
	case v.err != nil:
		status = "Load failed · [r] Retry"
	case v.jumpLetter != "":
		status = "Finding " + v.jumpLetter + " · [esc] Cancel"
	case v.jumpEnd:
		status = "Finding last item · [esc] Cancel"
	case v.isLoading:
		status = "Loading…"
	}
	footer := count + " · [g] A–Z"
	if v.server.Shared {
		footer = "Shared · " + v.title + " · " + footer
	}
	if status != "" {
		footer += " · " + status
	}
	return body + "\n" + lipgloss.NewStyle().Foreground(v.theme.BrightBlack()).
		Render(ui.Ellipsis(footer, ui.GetLayout().InnerWidth()))
}

func (v *MediaView) ActionRows() int {
	return 1
}

func (v *MediaView) syncTableRows() {
	items := presenters.MapMetadata(v.allMetadata)
	rows := make([]table.Row, len(items))
	for i, item := range items {
		rows[i] = item.ToRow()
	}
	v.table.SetRows(rows)
}

func (v *MediaView) MediaServer() config.ServerTarget { return v.server }

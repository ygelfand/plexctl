package view

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/cache"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
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
	sectionID   string
	title       string
	table       table.Model
	posterGrid  *poster.PosterGrid
	theme       tint.Tint
	err         error
	totalItems  int
	loadedItems int
	isLoading   bool
	allMetadata []components.Metadata
	viewMode    ViewMode

	details detail.DetailManager
}

func NewMediaView(sectionID, title string, theme tint.Tint) *MediaView {
	columns := []table.Column{
		{Title: "TITLE", Width: 50},
		{Title: "TYPE", Width: 15},
		{Title: "YEAR", Width: 10},
		{Title: "WATCHED", Width: 15},
	}

	t := ui.NewTable(columns, theme)

	cfg := config.Get()
	mode := config.ViewModeList
	if cfg.DefaultViewMode != "" {
		mode = cfg.DefaultViewMode
	}

	_, server, ok := cfg.GetActiveServer()
	if ok {
		if opts, ok := server.Libraries.Settings[sectionID]; ok && opts.ViewMode != "" {
			mode = opts.ViewMode
		}
	}

	vMode := ViewModeList
	if mode == config.ViewModePoster {
		vMode = ViewModePoster
	}

	return &MediaView{
		sectionID:   sectionID,
		title:       title,
		table:       t,
		posterGrid:  poster.NewPosterGrid(nil, theme),
		theme:       theme,
		viewMode:    vMode,
		allMetadata: []components.Metadata{},
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
	v.isLoading = true
	return v.fetchPage(0)
}

func (v *MediaView) fetchPage(start int) tea.Cmd {
	return func() tea.Msg {
		slog.Debug("MediaView: fetchPage started", "section", v.sectionID, "start", start)
		client, err := plex.NewClient()
		if err != nil {
			return err
		}

		cfg := config.Get()
		serverID, _, _ := cfg.GetActiveServer()
		cm, _ := cache.Get(cfg.CacheDir)

		req := operations.ListContentRequest{
			SectionID:           v.sectionID,
			XPlexContainerStart: ui.Ptr(start),
			XPlexContainerSize:  ui.Ptr(100),
		}

		var body components.MediaContainerWithMetadata
		err = cache.AutoCache(cm, serverID, req, plex.LibraryCacheTTL, &body, func() (*components.MediaContainerWithMetadata, error) {
			res, err := client.SDK.Content.ListContent(context.Background(), req)
			if err != nil {
				return nil, err
			}
			if res.MediaContainerWithMetadata == nil {
				return nil, fmt.Errorf("no metadata")
			}
			return res.MediaContainerWithMetadata, nil
		})
		if err != nil {
			slog.Error("MediaView fetch error", "error", err)
			return err
		}

		if body.MediaContainer == nil {
			return fmt.Errorf("no media container found")
		}

		total := 0
		if body.MediaContainer.TotalSize != nil {
			total = int(*body.MediaContainer.TotalSize)
		} else {
			total = len(body.MediaContainer.Metadata)
		}

		slog.Debug("MediaView: fetchPage finished", "section", v.sectionID, "count", len(body.MediaContainer.Metadata), "total", total)

		return ui.MediaPageMsg{Metadata: body.MediaContainer.Metadata, Total: total}
	}
}

func (v *MediaView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if v.details.Active() {
		// Parent-level updates still happen even if detail is active (like window size)
		switch m := msg.(type) {
		case tea.WindowSizeMsg:
			v.table.SetHeight(ui.GetTableHeight(ui.GetLayout().TotalHeight()))
			v.table.SetWidth(ui.GetLayout().MainAreaContentWidth())
			v.posterGrid.Update(m)
		case ui.ThemeChangedMsg:
			v.theme = m.Theme
			v.posterGrid.Theme = v.theme
			ui.UpdateTableTheme(&v.table, ui.GetLayout().MainAreaContentWidth(), ui.GetLayout().TotalHeight())
		}

		cmd, handled := v.details.Update(msg)
		if handled {
			return v, cmd
		}
	}

	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.table.SetHeight(ui.GetTableHeight(ui.GetLayout().TotalHeight()))
		v.table.SetWidth(ui.GetLayout().MainAreaContentWidth())
		v.posterGrid.Update(msg)
	case ui.ThemeChangedMsg:
		v.theme = msg.Theme
		v.posterGrid.Theme = v.theme
		ui.UpdateTableTheme(&v.table, ui.GetLayout().MainAreaContentWidth(), ui.GetLayout().TotalHeight())
	case ui.MediaPageMsg:
		v.isLoading = false
		v.totalItems = msg.Total
		v.allMetadata = append(v.allMetadata, msg.Metadata...)
		v.loadedItems = len(v.allMetadata)
		v.syncTableRows()
		cmds = append(cmds, v.posterGrid.SetItems(v.allMetadata))
		return v, tea.Batch(cmds...)
	case poster.PosterLoadedMsg:
		_, cmd := v.posterGrid.Update(msg)
		return v, cmd
	case error:
		v.err = msg
	case tea.KeyMsg:
		switch msg.String() {
		case "v":
			cfg := config.Get()
			id, server, ok := cfg.GetActiveServer()

			if v.viewMode == ViewModeList {
				v.viewMode = ViewModePoster
				v.posterGrid.Cursor = v.table.Cursor()
				cmds = append(cmds, v.posterGrid.Init())
			} else {
				v.viewMode = ViewModeList
				v.table.SetCursor(v.posterGrid.Cursor)
			}

			if ok {
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

	// Lazy loading
	if !v.isLoading && v.loadedItems > 0 && v.loadedItems < v.totalItems {
		currIdx := v.table.Cursor()
		if v.viewMode == ViewModePoster {
			currIdx = v.posterGrid.Cursor
		}

		if currIdx > v.loadedItems-20 {
			v.isLoading = true
			cmds = append(cmds, v.fetchPage(v.loadedItems))
		}
	}

	return v, tea.Batch(cmds...)
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
		dv = detail.NewShowDetailView(ratingKey, v.theme)
	case "episode":
		dv = detail.NewEpisodeDetailView(ratingKey, v.theme)
	default:
		dv = detail.NewMovieDetailView(ratingKey, v.theme)
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
		{Key: "v", Desc: "Toggle View"},
		{Key: "j/up", Desc: "Move Up"},
		{Key: "k/down", Desc: "Move Down"},
	}
}

func (v *MediaView) View() string {
	if v.details.Active() {
		return v.details.ViewContent()
	}

	if v.viewMode == ViewModeList {
		return lipgloss.NewStyle().Render(v.table.View())
	}

	return v.posterGrid.View()
}

func (v *MediaView) syncTableRows() {
	rows := make([]table.Row, len(v.allMetadata))
	for i, meta := range v.allMetadata {
		year := ""
		if meta.Year != nil {
			year = fmt.Sprintf("%d", *meta.Year)
		}
		rows[i] = table.Row{
			meta.Title,
			meta.Type,
			year,
			detail.RenderWatchedStatus(&meta),
		}
	}
	v.table.SetRows(rows)
}

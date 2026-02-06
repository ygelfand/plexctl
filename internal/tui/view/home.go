package view

import (
	"context"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/tui/view/detail"
	"github.com/ygelfand/plexctl/internal/tui/widget/poster"
	"github.com/ygelfand/plexctl/internal/ui"
)

type HomeView struct {
	hubs       []components.Hub
	lists      []*poster.PosterList
	activeList int
	theme      tint.Tint
	loading    bool

	details  detail.DetailManager
	viewport viewport.Model
}

func NewHomeView(theme tint.Tint) *HomeView {
	return &HomeView{
		theme:    theme,
		loading:  true,
		viewport: viewport.New(0, 0),
	}
}

type homeHubsMsg []components.Hub

func (v *HomeView) Init() tea.Cmd {
	return v.fetchHubs
}

func (v *HomeView) Refresh() tea.Cmd {
	if v.details.Active() {
		if refresher, ok := v.details.View.(ui.Refreshable); ok {
			return refresher.Refresh()
		}
	}
	v.loading = true
	return v.fetchHubs
}

func (v *HomeView) fetchHubs() tea.Msg {
	hubs, err := plex.GetHomeHubs(context.Background())
	if err != nil {
		return err
	}
	return homeHubsMsg(hubs)
}

func (v *HomeView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if v.details.Active() {
		cmd, handled := v.details.Update(msg)
		if handled {
			return v, cmd
		}
	}

	var cmds []tea.Cmd
	shouldRedraw := false

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		layout := ui.GetLayout()
		v.viewport.Width = layout.InnerWidth()
		v.viewport.Height = layout.ContentHeight()
		shouldRedraw = true
	case ui.ThemeChangedMsg:
		v.theme = msg.Theme
		for _, l := range v.lists {
			l.Theme = msg.Theme
		}
		shouldRedraw = true
	case homeHubsMsg:
		v.loading = false
		v.hubs = msg

		var newLists []*poster.PosterList
		for i, hub := range v.hubs {
			if len(hub.Metadata) > 0 {
				title := "Hub"
				if hub.Title != nil {
					title = *hub.Title
				}

				// Try to find existing list to preserve state
				var existing *poster.PosterList
				for _, l := range v.lists {
					if l.Title == title {
						existing = l
						break
					}
				}

				if existing != nil {
					cmds = append(cmds, existing.SetItems(hub.Metadata))
					newLists = append(newLists, existing)
				} else {
					l := poster.NewPosterList(i, title, hub.Metadata, v.theme)
					newLists = append(newLists, l)
					cmds = append(cmds, l.Init())
				}
			}
		}
		v.lists = newLists

		if len(v.lists) > 0 {
			if v.activeList >= len(v.lists) {
				v.activeList = 0
			}
			// Reset all to inactive first
			for _, l := range v.lists {
				l.Active = false
			}
			v.lists[v.activeList].Active = true
			v.syncViewport()
		}
		shouldRedraw = true
	case poster.PosterLoadedMsg:
		shouldRedraw = true
	case poster.ItemSelectedMsg:
		meta := msg.Metadata
		ratingKey := ""
		if meta.RatingKey != nil {
			ratingKey = *meta.RatingKey
		}

		var dv tea.Model
		switch meta.Type {
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

		return v, v.details.Set(dv)

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if v.activeList > 0 {
				v.lists[v.activeList].Active = false
				v.activeList--
				v.lists[v.activeList].Active = true
				v.syncViewport()
				shouldRedraw = true
			}
		case "down", "j":
			if v.activeList < len(v.lists)-1 {
				v.lists[v.activeList].Active = false
				v.activeList++
				v.lists[v.activeList].Active = true
				v.syncViewport()
				shouldRedraw = true
			}
		case "left", "h", "right", "l":
			shouldRedraw = true
		case "r":
			return v, v.Refresh()
		}
	}

	for _, l := range v.lists {
		_, cmd := l.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	if shouldRedraw {
		v.updateViewportContent()
	}

	return v, tea.Batch(cmds...)
}

func (v *HomeView) syncViewport() {
	targetLine := v.activeList * (ui.PosterTotalHeight + 1)
	v.viewport.SetYOffset(targetLine)
}

func (v *HomeView) GetSelectedMetadata() *components.Metadata {
	if v.details.Active() {
		if provider, ok := v.details.View.(ui.PlayableProvider); ok {
			return provider.GetSelectedMetadata()
		}
		return nil
	}

	if v.activeList >= 0 && v.activeList < len(v.lists) {
		l := v.lists[v.activeList]
		if l.Cursor >= 0 && l.Cursor < len(l.Items) {
			return &l.Items[l.Cursor].Metadata
		}
	}
	return nil
}

func (v *HomeView) updateViewportContent() {
	if len(v.lists) == 0 {
		return
	}

	var sections []string
	for _, l := range v.lists {
		sections = append(sections, l.View())
	}
	v.viewport.SetContent(lipgloss.JoinVertical(lipgloss.Left, sections...))
}

func (v *HomeView) View() string {
	if v.details.Active() {
		return v.details.ViewContent()
	}
	if v.loading {
		return "\n  ⌛ Loading Dashboard..."
	}
	if len(v.lists) == 0 {
		return "\n  No promoted hubs found."
	}

	return lipgloss.NewStyle().
		Width(ui.GetLayout().InnerWidth()).
		Render(v.viewport.View())
}

func (v *HomeView) HelpKeys() []ui.HelpKey {
	if v.details.Active() {
		if provider, ok := v.details.View.(ui.HelpProvider); ok {
			return provider.HelpKeys()
		}
	}
	return []ui.HelpKey{
		{Key: "↑/↓", Desc: "Switch Section"},
		{Key: "←/→", Desc: "Browse Items"},
		{Key: "enter", Desc: "View Details"},
	}
}

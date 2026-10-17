package view

import (
	"context"
	"fmt"

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
	err        error
	request    viewRequest

	details  detail.DetailManager
	viewport viewport.Model
}

func NewHomeView(theme tint.Tint) *HomeView {
	return &HomeView{
		request:  newViewRequest(),
		theme:    theme,
		loading:  true,
		viewport: viewport.New(0, 0),
	}
}

func (v *HomeView) CanJumpLetters() bool {
	target, ok := v.details.View.(ui.LetterJumper)
	return ok && target.CanJumpLetters()
}

func (v *HomeView) AlphabetRail() *ui.AlphabetRail {
	if target, ok := v.details.View.(ui.AlphabetNavigator); ok {
		return target.AlphabetRail()
	}
	return nil
}

func (v *HomeView) JumpToLetter(letter string) tea.Cmd {
	if target, ok := v.details.View.(ui.LetterJumper); ok {
		return target.JumpToLetter(letter)
	}
	return nil
}

func (v *HomeView) Init() tea.Cmd {
	return v.Refresh()
}

func (v *HomeView) Refresh() tea.Cmd {
	if v.details.Active() {
		if refresher, ok := v.details.View.(ui.Refreshable); ok {
			return refresher.Refresh()
		}
	}
	v.loading = true
	v.err = nil
	return v.request.start(v, func(ctx context.Context) (any, error) {
		return plex.GetHomeHubs(ctx)
	})
}

func (v *HomeView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if result, ok := msg.(DataLoadedMsg); ok && !v.request.accepts(v, result) {
		return v, nil
	}
	if posterMsg, ok := msg.(poster.PosterLoadedMsg); ok && v.details.Active() {
		for _, list := range v.lists {
			list.Update(posterMsg)
		}
	}
	_, homeResult := msg.(DataLoadedMsg)
	if v.details.Active() && !homeResult {
		cmd, handled := v.details.Update(msg)
		if handled {
			return v, cmd
		}
	}

	var cmds []tea.Cmd
	shouldRedraw := false

	switch msg := msg.(type) {
	case tea.MouseMsg:
		if delta := ui.WheelDelta(msg); delta != 0 && !msg.Shift &&
			msg.Button != tea.MouseButtonWheelLeft && msg.Button != tea.MouseButtonWheelRight {
			v.viewport.SetYOffset(v.viewport.YOffset + delta*3)
			return v, nil
		}
		y := -v.viewport.YOffset
		for i, l := range v.lists {
			height := lipgloss.Height(l.View())
			local, inside := (ui.Rect{Y: y, Width: v.viewport.Width, Height: height}).Local(msg)
			if inside {
				if ui.LeftClick(msg) || ui.WheelDelta(msg) != 0 {
					v.lists[v.activeList].Active = false
					v.activeList = i
					l.Active = true
				}
				_, cmd := l.Update(local)
				v.updateViewportContent()
				return v, cmd
			}
			y += height
		}
		return v, nil
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
	case DataLoadedMsg:
		v.loading = false
		v.err = msg.err
		if msg.err != nil {
			return v, nil
		}
		v.hubs = msg.data.([]components.Hub)

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
					l := poster.NewPosterList(i, title, hub.Metadata, v.theme, v.request.server)
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
			dv = detail.NewShowDetailView(ratingKey, v.theme, v.request.server)
		case "episode":
			dv = detail.NewEpisodeDetailView(ratingKey, v.theme, v.request.server)
		case "artist", "album", "track":
			dv = detail.NewMusicDetailView(ratingKey, v.theme, v.request.server)
		default:
			dv = detail.NewMovieDetailView(ratingKey, v.theme, v.request.server)
		}

		layout := ui.GetLayout()
		if sizer, ok := dv.(interface{ SetSize(int, int) }); ok {
			sizer.SetSize(layout.InnerWidth(), layout.ContentHeight())
		}

		return v, v.details.Set(dv)

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			cmds = append(cmds, v.moveList(-1))
			shouldRedraw = true
		case "down", "j":
			cmds = append(cmds, v.moveList(1))
			shouldRedraw = true
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

func (v *HomeView) moveList(delta int) tea.Cmd {
	next := v.activeList + delta
	if next < 0 || next >= len(v.lists) {
		return nil
	}

	current := v.lists[v.activeList]
	column := current.Cursor - current.Offset
	current.Active = false
	v.activeList = next
	target := v.lists[next]
	target.Active = true
	// Each row scrolls independently. Follow the visible column, clamping to
	// the nearest poster when the destination row has fewer items.
	target.Cursor = min(target.Offset+column, len(target.Items)-1)
	cmd := target.Sync()
	v.syncViewport()
	return cmd
}

func (v *HomeView) syncViewport() {
	targetLine := 0
	for i := 0; i < v.activeList; i++ {
		targetLine += lipgloss.Height(v.lists[i].View())
	}
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
	if v.err != nil {
		return fmt.Sprintf("Unable to load dashboard: %v\n[r] Retry", v.err)
	}
	if len(v.lists) == 0 {
		return "\n  No promoted hubs found."
	}

	return lipgloss.NewStyle().
		Width(ui.GetLayout().InnerWidth()).
		Render(v.viewport.View())
}

func (v *HomeView) ActionRows() int {
	if v.details.Active() {
		return 1
	}
	return 0
}

func (v *HomeView) HelpKeys() []ui.HelpKey {
	if v.details.Active() {
		if provider, ok := v.details.View.(ui.HelpProvider); ok {
			return provider.HelpKeys()
		}
	}
	return []ui.HelpKey{
		{Key: "j/k, ↓/↑", Desc: "Switch rows in the same column"},
		{Key: "h/l, ←/→", Desc: "Browse Items"},
		{Key: "enter", Desc: "View Details"},
	}
}

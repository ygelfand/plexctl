package poster

import (
	"github.com/LukeHagar/plexgo/models/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/tui/artwork"
	"github.com/ygelfand/plexctl/internal/ui"
)

type PosterList struct {
	server config.ServerTarget
	ID     int
	Title  string
	Items  []*PosterItem
	Cursor int
	Offset int
	Theme  tint.Tint
	Active bool
}

func NewPosterList(id int, title string, metadata []components.Metadata, theme tint.Tint, servers ...config.ServerTarget) *PosterList {
	server := config.MediaServer(servers...)
	items := make([]*PosterItem, len(metadata))
	for i, m := range metadata {
		items[i] = &PosterItem{Metadata: m, Loading: true}

		rk := ""
		if m.RatingKey != nil {
			rk = *m.RatingKey
		}
		if rk != "" {
			if cached, ok := artwork.Cached(server.CacheNamespace()+"/"+rk, ui.PosterWidth); ok {
				items[i].Poster = cached
				items[i].Loading = false
			}
		}
	}

	return &PosterList{
		server: server,
		ID:     id,
		Title:  title,
		Items:  items,
		Theme:  theme,
	}
}

func (m *PosterList) Init() tea.Cmd {
	return m.fetchVisible()
}

func (m *PosterList) SetItems(metadata []components.Metadata) tea.Cmd {
	newItems := make([]*PosterItem, len(metadata))
	for i, meta := range metadata {
		if i < len(m.Items) && m.Items[i].Metadata.RatingKey != nil && meta.RatingKey != nil && *m.Items[i].Metadata.RatingKey == *meta.RatingKey {
			newItems[i] = m.Items[i]
		} else {
			newItems[i] = &PosterItem{Metadata: meta, Loading: true}
			rk := ""
			if meta.RatingKey != nil {
				rk = *meta.RatingKey
			}
			if rk != "" {
				if cached, ok := artwork.Cached(m.server.CacheNamespace()+"/"+rk, ui.PosterWidth); ok {
					newItems[i].Poster = cached
					newItems[i].Loading = false
				}
			}
		}
	}
	m.Items = newItems
	if m.Cursor >= len(m.Items) {
		m.Cursor = max(len(m.Items)-1, 0)
	}
	return tea.Batch(m.fixScroll(), m.fetchVisible())
}

func (m *PosterList) Sync() tea.Cmd {
	return m.fixScroll()
}

func (m *PosterList) fetchVisible() tea.Cmd {
	var cmds []tea.Cmd
	cols := ui.GetLayout().PosterColumns()

	start := m.Offset
	end := min(m.Offset+cols+2, len(m.Items))

	for i := start; i < end; i++ {
		if m.Items[i].Loading && !m.Items[i].Fetching {
			m.Items[i].Fetching = true
			cmds = append(cmds, FetchPoster(m.ID, i, m.Items[i].Metadata, m.server))
		}
	}
	return tea.Batch(cmds...)
}

func (m *PosterList) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case PosterLoadedMsg:
		if msg.ServerID != "" && msg.ServerID != m.server.CacheNamespace() {
			return m, nil
		}
		if msg.ListID == m.ID && msg.Index >= 0 && msg.Index < len(m.Items) {
			if msg.RatingKey != ui.PtrToString(m.Items[msg.Index].Metadata.RatingKey) {
				return m, nil
			}
			m.Items[msg.Index].Poster = msg.View
			m.Items[msg.Index].Loading = false
			m.Items[msg.Index].Fetching = false
		}
		return m, nil
	case tea.WindowSizeMsg:
		return m, m.fixScroll()
	}

	if !m.Active {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.MouseMsg:
		if delta := ui.WheelDelta(msg); delta != 0 {
			key := "right"
			if delta < 0 {
				key = "left"
			}
			return m.Update(ui.KeyPress(key))
		}
		titleHeight := lipgloss.Height(ui.TitleStyle(m.Theme).Render(m.Title))
		if ui.LeftClick(msg) && msg.X >= 0 && msg.Y >= titleHeight && msg.Y < titleHeight+ui.PosterTotalHeight {
			column := msg.X / ui.PosterStepWidth
			index := m.Offset + column
			if column < ui.GetLayout().PosterColumns() && msg.X%ui.PosterStepWidth < ui.PosterWidth+ui.PosterBorder && index < len(m.Items) {
				m.Cursor = index
				return m.Update(ui.KeyPress("enter"))
			}
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if m.Cursor >= 0 && m.Cursor < len(m.Items) {
				metadata := m.Items[m.Cursor].Metadata
				return m, func() tea.Msg {
					return ItemSelectedMsg{Metadata: metadata}
				}
			}
		case "left", "h":
			if m.Cursor > 0 {
				m.Cursor--
				return m, m.fixScroll()
			}
		case "right", "l":
			if m.Cursor < len(m.Items)-1 {
				m.Cursor++
				return m, m.fixScroll()
			}
		}
	}
	return m, nil
}

func (m *PosterList) fixScroll() tea.Cmd {
	cols := ui.GetLayout().PosterColumns()

	if m.Cursor < m.Offset {
		m.Offset = m.Cursor
	} else if m.Cursor >= m.Offset+cols {
		m.Offset = m.Cursor - cols + 1
	}

	return m.fetchVisible()
}

func (m *PosterList) View() string {
	if len(m.Items) == 0 {
		return ""
	}

	cols := ui.GetLayout().PosterColumns()

	var itemViews []string
	end := min(m.Offset+cols, len(m.Items))

	for i := m.Offset; i < end; i++ {
		itemViews = append(itemViews, RenderPosterItem(m.Items[i], i == m.Cursor, m.Active, m.Theme))
	}

	row := lipgloss.JoinHorizontal(lipgloss.Top, itemViews...)

	return lipgloss.JoinVertical(lipgloss.Left,
		ui.TitleStyle(m.Theme).Render(m.Title),
		row,
	)
}

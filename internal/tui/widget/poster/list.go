package poster

import (
	"github.com/LukeHagar/plexgo/models/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/ui"
)

type PosterList struct {
	ID     int
	Title  string
	Items  []*PosterItem
	Cursor int
	Offset int
	Theme  tint.Tint
	Active bool
}

func NewPosterList(id int, title string, metadata []components.Metadata, theme tint.Tint) *PosterList {
	items := make([]*PosterItem, len(metadata))
	for i, m := range metadata {
		items[i] = &PosterItem{Metadata: m, Loading: true}

		rk := ""
		if m.RatingKey != nil {
			rk = *m.RatingKey
		}
		if rk != "" {
			if cached, ok := plex.GetCachedPoster(rk, ui.PosterWidth); ok {
				items[i].Poster = cached
				items[i].Loading = false
			}
		}
	}

	return &PosterList{
		ID:    id,
		Title: title,
		Items: items,
		Theme: theme,
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
				if cached, ok := plex.GetCachedPoster(rk, ui.PosterWidth); ok {
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
		if m.Items[i].Loading {
			cmds = append(cmds, FetchPoster(m.ID, i, m.Items[i].Metadata))
		}
	}
	return tea.Batch(cmds...)
}

func (m *PosterList) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case PosterLoadedMsg:
		if msg.ListID == m.ID && msg.Index >= 0 && msg.Index < len(m.Items) {
			m.Items[msg.Index].Poster = msg.View
			m.Items[msg.Index].Loading = false
		}
		return m, nil
	case tea.WindowSizeMsg:
		return m, m.fetchVisible()
	}

	if !m.Active {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if m.Cursor >= 0 && m.Cursor < len(m.Items) {
				return m, func() tea.Msg {
					return ItemSelectedMsg{Metadata: m.Items[m.Cursor].Metadata}
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
	end := m.Offset + cols
	if end > len(m.Items) {
		end = len(m.Items)
	}

	for i := m.Offset; i < end; i++ {
		itemViews = append(itemViews, RenderPosterItem(m.Items[i], i == m.Cursor, m.Active, m.Theme))
	}

	row := lipgloss.JoinHorizontal(lipgloss.Top, itemViews...)

	return lipgloss.JoinVertical(lipgloss.Left,
		ui.TitleStyle(m.Theme).Render(m.Title),
		row,
	)
}

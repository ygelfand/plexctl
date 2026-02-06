package poster

import (
	"github.com/LukeHagar/plexgo/models/components"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/ui"
)

type PosterGrid struct {
	Items    []*PosterItem
	Cursor   int
	Theme    tint.Tint
	viewport viewport.Model
}

func NewPosterGrid(metadata []components.Metadata, theme tint.Tint) *PosterGrid {
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

	return &PosterGrid{
		Items:    items,
		Theme:    theme,
		viewport: viewport.New(0, 0),
	}
}

func (m *PosterGrid) Init() tea.Cmd {
	return m.fetchVisible()
}

func (m *PosterGrid) SetItems(metadata []components.Metadata) tea.Cmd {
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
	return m.fetchVisible()
}

func (m *PosterGrid) fetchVisible() tea.Cmd {
	layout := ui.GetLayout()
	cols := layout.PosterColumns()
	if cols <= 0 {
		return nil
	}

	var cmds []tea.Cmd
	rowsVisible := layout.ContentHeight() / ui.PosterTotalHeight
	if rowsVisible <= 0 {
		rowsVisible = 3
	}

	startRow := m.viewport.YOffset / ui.PosterTotalHeight
	startIdx := startRow * cols
	endIdx := (startRow + rowsVisible + 2) * cols
	if endIdx > len(m.Items) {
		endIdx = len(m.Items)
	}

	for i := startIdx; i < endIdx; i++ {
		if m.Items[i].Loading {
			cmds = append(cmds, FetchPoster(0, i, m.Items[i].Metadata))
		}
	}
	return tea.Batch(cmds...)
}

func (m *PosterGrid) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		layout := ui.GetLayout()
		m.viewport.Width = layout.MainAreaContentWidth()
		m.viewport.Height = layout.ContentHeight()
		cmds = append(cmds, m.fetchVisible())

	case PosterLoadedMsg:
		if msg.Index >= 0 && msg.Index < len(m.Items) {
			m.Items[msg.Index].Poster = msg.View
			m.Items[msg.Index].Loading = false
		}

	case tea.KeyMsg:
		cols := ui.GetLayout().PosterColumns()
		switch msg.String() {
		case "up", "k":
			if m.Cursor >= cols {
				m.Cursor -= cols
				cmds = append(cmds, m.syncScroll())
			}
		case "down", "j":
			if m.Cursor+cols < len(m.Items) {
				m.Cursor += cols
				cmds = append(cmds, m.syncScroll())
			}
		case "left", "h":
			if m.Cursor > 0 {
				m.Cursor--
				cmds = append(cmds, m.syncScroll())
			}
		case "right", "l":
			if m.Cursor < len(m.Items)-1 {
				m.Cursor++
				cmds = append(cmds, m.syncScroll())
			}
		case "enter":
			if m.Cursor >= 0 && m.Cursor < len(m.Items) {
				return m, func() tea.Msg {
					return ItemSelectedMsg{Metadata: m.Items[m.Cursor].Metadata}
				}
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *PosterGrid) syncScroll() tea.Cmd {
	layout := ui.GetLayout()
	cols := layout.PosterColumns()
	row := m.Cursor / cols
	targetY := row * ui.PosterTotalHeight

	if targetY < m.viewport.YOffset {
		m.viewport.SetYOffset(targetY)
	} else if targetY+ui.PosterTotalHeight > m.viewport.YOffset+m.viewport.Height {
		m.viewport.SetYOffset(targetY - m.viewport.Height + ui.PosterTotalHeight)
	}

	return m.fetchVisible()
}

func (m *PosterGrid) View() string {
	if len(m.Items) == 0 {
		return ""
	}

	layout := ui.GetLayout()
	cols := layout.PosterColumns()

	var rows []string
	var currentRow []string

	for i, item := range m.Items {
		currentRow = append(currentRow, RenderPosterItem(item, i == m.Cursor, true, m.Theme))

		if len(currentRow) == cols || i == len(m.Items)-1 {
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, currentRow...))
			currentRow = nil
		}
	}

	m.viewport.SetContent(lipgloss.JoinVertical(lipgloss.Left, rows...))
	return m.viewport.View()
}

package poster

import (
	"strings"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/tui/artwork"
	"github.com/ygelfand/plexctl/internal/ui"
)

type PosterGrid struct {
	server   config.ServerTarget
	Items    []*PosterItem
	Cursor   int
	Theme    tint.Tint
	viewport viewport.Model
}

func NewPosterGrid(metadata []components.Metadata, theme tint.Tint, servers ...config.ServerTarget) *PosterGrid {
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

	return &PosterGrid{
		server:   server,
		Items:    items,
		Theme:    theme,
		viewport: viewport.New(0, 0),
	}
}

func (m *PosterGrid) Init() tea.Cmd {
	return m.fetchVisible()
}

// SetCursor scrolls to an absolute item, including items on newly appended pages.
func (m *PosterGrid) SetCursor(index int) tea.Cmd {
	m.Cursor = max(0, min(index, len(m.Items)-1))
	m.View()
	return m.syncScroll()
}

// SetSize lets embedded grids, such as an artist's albums, use their own
// content area instead of the entire library viewport.
func (m *PosterGrid) SetSize(width, height int) {
	m.viewport.Width, m.viewport.Height = max(1, width), max(1, height)
	m.View()
	m.scrollToCursor()
}

func (m *PosterGrid) columns() int {
	width := m.viewport.Width
	if width <= 0 {
		width = ui.GetLayout().InnerWidth()
	}
	return max(1, width/ui.PosterStepWidth)
}

// LastVisibleIndex follows the viewport even when the mouse scrolls without
// moving the keyboard selection. Libraries use it to load the next page.
func (m *PosterGrid) LastVisibleIndex() int {
	rows := (m.viewport.YOffset + max(1, m.viewport.Height) + ui.PosterTotalHeight - 1) / ui.PosterTotalHeight
	return min(len(m.Items)-1, rows*m.columns()-1)
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
				if cached, ok := artwork.Cached(m.server.CacheNamespace()+"/"+rk, ui.PosterWidth); ok {
					newItems[i].Poster = cached
					newItems[i].Loading = false
				}
			}
		}
	}
	m.Items = newItems
	m.Cursor = min(m.Cursor, max(0, len(m.Items)-1))
	return m.fetchVisible()
}

func (m *PosterGrid) fetchVisible() tea.Cmd {
	cols := m.columns()
	if cols <= 0 {
		return nil
	}

	var cmds []tea.Cmd
	rowsVisible := m.viewport.Height / ui.PosterTotalHeight
	if rowsVisible <= 0 {
		rowsVisible = 3
	}

	startRow := m.viewport.YOffset / ui.PosterTotalHeight
	startIdx := startRow * cols
	endIdx := min((startRow+rowsVisible+2)*cols, len(m.Items))

	for i := startIdx; i < endIdx; i++ {
		if m.Items[i].Loading && !m.Items[i].Fetching {
			m.Items[i].Fetching = true
			cmds = append(cmds, FetchPoster(0, i, m.Items[i].Metadata, m.server))
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
		m.View() // Reflow the viewport before restoring the selected row.
		cmds = append(cmds, m.syncScroll())
		cmds = append(cmds, m.fetchVisible())

	case PosterLoadedMsg:
		if msg.ServerID != "" && msg.ServerID != m.server.CacheNamespace() {
			return m, nil
		}
		if msg.Index >= 0 && msg.Index < len(m.Items) {
			if msg.RatingKey != ui.PtrToString(m.Items[msg.Index].Metadata.RatingKey) {
				return m, nil
			}
			m.Items[msg.Index].Poster = msg.View
			m.Items[msg.Index].Loading = false
			m.Items[msg.Index].Fetching = false
		}
	case tea.MouseMsg:
		if _, inside := (ui.Rect{Width: m.viewport.Width, Height: m.viewport.Height}).Local(msg); !inside {
			return m, nil
		}
		if delta := ui.WheelDelta(msg); delta != 0 {
			m.viewport.SetYOffset(m.viewport.YOffset + delta*3)
			return m, m.fetchVisible()
		}
		if ui.LeftClick(msg) {
			cols := m.columns()
			column := msg.X / ui.PosterStepWidth
			row := (msg.Y + m.viewport.YOffset) / ui.PosterTotalHeight
			index := row*cols + column
			if column < cols && msg.X%ui.PosterStepWidth < ui.PosterWidth+ui.PosterBorder && index < len(m.Items) {
				m.Cursor = index
				return m.Update(ui.KeyPress("enter"))
			}
		}

	case tea.KeyMsg:
		cols := m.columns()
		switch msg.String() {
		case "home":
			return m, m.SetCursor(0)
		case "end":
			return m, m.SetCursor(len(m.Items) - 1)
		case "pgup", "ctrl+u":
			return m, m.SetCursor(m.Cursor - max(1, m.viewport.Height/ui.PosterTotalHeight)*cols)
		case "pgdown", "ctrl+d":
			return m, m.SetCursor(m.Cursor + max(1, m.viewport.Height/ui.PosterTotalHeight)*cols)
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
				metadata := m.Items[m.Cursor].Metadata
				return m, func() tea.Msg {
					return ItemSelectedMsg{Metadata: metadata}
				}
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *PosterGrid) syncScroll() tea.Cmd {
	m.scrollToCursor()
	return m.fetchVisible()
}

func (m *PosterGrid) scrollToCursor() {
	cols := m.columns()
	row := m.Cursor / cols
	targetY := row * ui.PosterTotalHeight

	if targetY < m.viewport.YOffset {
		m.viewport.SetYOffset(targetY)
	} else if targetY+ui.PosterTotalHeight > m.viewport.YOffset+m.viewport.Height {
		m.viewport.SetYOffset(targetY - m.viewport.Height + ui.PosterTotalHeight)
	}

}

func (m *PosterGrid) View() string {
	if len(m.Items) == 0 {
		return ""
	}

	cols := m.columns()

	rowCount := (len(m.Items) + cols - 1) / cols
	startRow := m.viewport.YOffset / ui.PosterTotalHeight
	endRow := min(rowCount, (m.viewport.YOffset+max(1, m.viewport.Height)+ui.PosterTotalHeight-1)/ui.PosterTotalHeight)
	rows := make([]string, rowCount)
	// Only visible posters need rendering, even with thousands of artists and
	// a 20 FPS audio visualizer. Blank offscreen rows retain scroll geometry.
	for row := range rowCount {
		if row < startRow || row >= endRow {
			rows[row] = strings.Repeat("\n", ui.PosterTotalHeight-1)
			continue
		}
		items := make([]string, 0, cols)
		for i := row * cols; i < min((row+1)*cols, len(m.Items)); i++ {
			items = append(items, RenderPosterItem(m.Items[i], i == m.Cursor, true, m.Theme))
		}
		rows[row] = lipgloss.JoinHorizontal(lipgloss.Top, items...)
	}

	m.viewport.SetContent(lipgloss.JoinVertical(lipgloss.Left, rows...))
	return m.viewport.View()
}

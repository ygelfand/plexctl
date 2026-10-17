package search

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/sahilm/fuzzy"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/search"
	"github.com/ygelfand/plexctl/internal/ui"
)

type searchResultItem struct {
	entry search.IndexEntry
}

func (i searchResultItem) Title() string {
	title := i.entry.Title
	if i.entry.Year > 0 {
		title = fmt.Sprintf("%s (%d)", title, i.entry.Year)
	}
	return title
}
func (i searchResultItem) Description() string {
	return i.entry.Description()
}
func (i searchResultItem) FilterValue() string { return i.entry.Title }

type SearchOverlayModel struct {
	server     config.ServerTarget
	textInput  textinput.Model
	list       list.Model
	width      int
	height     int
	theme      tint.Tint
	allEntries []search.IndexEntry
	remote     []search.IndexEntry
	queryID    uint64
	cancel     context.CancelFunc
	loading    bool
	err        error
}

var nextQueryID atomic.Uint64

type queryReadyMsg struct{ id uint64 }
type resultsMsg struct {
	id      uint64
	entries []search.IndexEntry
	err     error
}

func NewSearchOverlayModel(theme tint.Tint, servers ...config.ServerTarget) *SearchOverlayModel {
	server := config.MediaServer(servers...)
	ti := textinput.New()
	ti.Placeholder = "Artists, albums, tracks, movies..."
	ti.Focus()
	ti.Prompt = " "

	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.KeyMap.Quit.SetKeys("esc")

	var entries []search.IndexEntry
	if server.CacheNamespace() == config.Get().MediaCacheNamespace() {
		entries, _ = search.GetIndex().Snapshot()
	}

	return &SearchOverlayModel{server: server,
		textInput:  ti,
		list:       l,
		theme:      theme,
		allEntries: entries,
	}
}

func (m *SearchOverlayModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m *SearchOverlayModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case queryReadyMsg:
		if msg.id != m.queryID {
			return m, nil
		}
		ctx, cancel := context.WithTimeout(m.server.Context(context.Background()), 10*time.Second)
		m.cancel = cancel
		query, id := m.textInput.Value(), m.queryID
		return m, func() tea.Msg {
			defer cancel()
			entries, err := search.Live(ctx, query)
			return resultsMsg{id: id, entries: entries, err: err}
		}
	case resultsMsg:
		if msg.id == m.queryID {
			m.loading = false
			m.remote = msg.entries
			m.err = msg.err
			m.runSearch()
		}
		return m, nil
	case tea.MouseMsg:
		msg.X -= 3
		msg.Y -= 4
		if ui.ListMouse(&m.list, msg) {
			return m.Update(ui.KeyPress("enter"))
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(max(1, min(m.width-8, max(m.width/2, 60))), max(5, min(m.height-8, 24)))
		m.textInput.Width = max(1, m.list.Width()-4)
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.Close()
			return nil, nil
		case "up", "down":
			var lCmd tea.Cmd
			m.list, lCmd = m.list.Update(msg)
			return m, lCmd
		case "enter":
			if m.list.SelectedItem() != nil {
				item := m.list.SelectedItem().(searchResultItem)
				m.Close()
				return nil, func() tea.Msg {
					return ui.SelectMediaMsg{Server: m.server,
						RatingKey: item.entry.RatingKey,
						Type:      item.entry.Type,
						SectionID: item.entry.SectionID,
					}
				}
			}
		}
	}

	var tiCmd tea.Cmd
	oldValue := m.textInput.Value()
	m.textInput, tiCmd = m.textInput.Update(msg)
	cmds = append(cmds, tiCmd)

	if m.textInput.Value() != oldValue {
		m.Close()
		m.queryID = nextQueryID.Add(1)
		m.remote = nil
		m.err = nil
		m.loading = strings.TrimSpace(m.textInput.Value()) != ""
		if m.loading {
			m.runSearch()
			id := m.queryID
			cmds = append(cmds, tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
				return queryReadyMsg{id: id}
			}))
		} else {
			m.list.SetItems(nil)
		}
	}

	var lCmd tea.Cmd
	m.list, lCmd = m.list.Update(msg)
	cmds = append(cmds, lCmd)

	return m, tea.Batch(cmds...)
}

func (m *SearchOverlayModel) runSearch() {
	query := m.textInput.Value()
	matches := fuzzy.FindFrom(query, entrySource(m.allEntries))

	items := []list.Item{}
	seen := map[string]bool{}
	for _, entry := range m.remote {
		items = append(items, searchResultItem{entry: entry})
		seen[entry.RatingKey] = true
	}
	for _, match := range matches {
		entry := m.allEntries[match.Index]
		if !seen[entry.RatingKey] {
			items = append(items, searchResultItem{entry: entry})
			seen[entry.RatingKey] = true
		}
		if len(items) >= 200 {
			break
		}
	}
	m.list.SetItems(items)
	m.list.ResetSelected() // Reset to the top/first page
}

type entrySource []search.IndexEntry

func (s entrySource) String(i int) string { return s[i].SearchableText() }
func (s entrySource) Len() int            { return len(s) }

func (m *SearchOverlayModel) View() string {
	accent := ui.Accent(m.theme)
	overlayStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Padding(1, 2).
		Background(lipgloss.Color("#111111"))

	status := "Search " + m.server.Name + " · [enter] Open · [esc] Back"
	switch {
	case m.loading:
		status = "Searching Plex…"
	case m.err != nil:
		status = "Server search unavailable; showing local matches. Retype to retry."
	case m.textInput.Value() != "":
		status = fmt.Sprintf("%d results · [enter] Open · Refine your search for more", len(m.list.Items()))
	}
	content := lipgloss.JoinVertical(lipgloss.Left,
		m.textInput.View(),
		"",
		m.list.View(),
		ui.Ellipsis(status, m.list.Width()),
	)

	return overlayStyle.Render(content)
}

func (m *SearchOverlayModel) Close() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
}

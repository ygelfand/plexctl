package search

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/sahilm/fuzzy"
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
	return fmt.Sprintf("[%s] %s", strings.ToUpper(i.entry.Type), i.entry.Library)
}
func (i searchResultItem) FilterValue() string { return i.entry.Title }

type SearchOverlayModel struct {
	textInput  textinput.Model
	list       list.Model
	width      int
	height     int
	theme      tint.Tint
	allEntries []search.IndexEntry
}

func NewSearchOverlayModel(theme tint.Tint) *SearchOverlayModel {
	ti := textinput.New()
	ti.Placeholder = "Search titles, actors, directors..."
	ti.Focus()
	ti.Prompt = " "

	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.KeyMap.Quit.SetKeys("esc")

	idx := search.GetIndex()

	return &SearchOverlayModel{
		textInput:  ti,
		list:       l,
		theme:      theme,
		allEntries: idx.Entries,
	}
}

func (m *SearchOverlayModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m *SearchOverlayModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetSize(max(m.width/2, 60), max(m.height/2, 20))
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return nil, nil
		case "enter":
			if m.list.SelectedItem() != nil {
				item := m.list.SelectedItem().(searchResultItem)
				return nil, func() tea.Msg {
					return ui.SelectMediaMsg{
						RatingKey: item.entry.RatingKey,
						Type:      item.entry.Type,
						SectionID: item.entry.SectionID,
					}
				}
			}
		}
	}

	var tiCmd tea.Cmd
	m.textInput, tiCmd = m.textInput.Update(msg)
	cmds = append(cmds, tiCmd)

	if m.textInput.Value() != "" {
		m.runSearch()
	} else {
		m.list.SetItems(nil)
	}

	var lCmd tea.Cmd
	m.list, lCmd = m.list.Update(msg)
	cmds = append(cmds, lCmd)

	return m, tea.Batch(cmds...)
}

func (m *SearchOverlayModel) runSearch() {
	query := m.textInput.Value()
	matches := fuzzy.FindFrom(query, entrySource(m.allEntries))

	var items []list.Item
	for _, match := range matches {
		items = append(items, searchResultItem{entry: m.allEntries[match.Index]})
		if len(items) >= 20 { // Limit results for performance
			break
		}
	}
	m.list.SetItems(items)
	m.list.ResetSelected() // Reset to the top/first page
}

type entrySource []search.IndexEntry

func (s entrySource) String(i int) string { return s[i].Title }
func (s entrySource) Len() int            { return len(s) }

func (m *SearchOverlayModel) View() string {
	accent := ui.Accent(m.theme)
	overlayStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Padding(1, 2).
		Background(lipgloss.Color("#111111"))

	content := lipgloss.JoinVertical(lipgloss.Left,
		m.textInput.View(),
		"",
		m.list.View(),
	)

	return overlayStyle.Render(content)
}

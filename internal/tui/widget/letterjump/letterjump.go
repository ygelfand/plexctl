package letterjump

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/ui"
)

// Model captures the second key of g + letter, so letters used for playback or
// quitting remain available as destinations without triggering those actions.
type Model struct {
	theme  tint.Tint
	rail   ui.AlphabetRail
	choice int
}

func New(theme tint.Tint) *Model {
	return &Model{theme: theme, rail: ui.NewAlphabetRail(12, 5, 9)}
}

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) choose(letter string) (tea.Model, tea.Cmd) {
	return nil, func() tea.Msg { return ui.JumpToLetterMsg{Letter: letter} }
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseMsg:
		if letter := m.rail.LetterAt(msg); letter != "" {
			return m.choose(letter)
		}
	case tea.KeyMsg:
		key := strings.ToUpper(msg.String())
		if len(key) == 1 && strings.Contains(ui.AlphabetLetters, key) {
			return m.choose(key)
		}
		switch msg.String() {
		case "esc", "ctrl+c":
			return nil, nil
		case "up", "left":
			m.choice = (m.choice + 26) % 27
		case "down", "right", "tab":
			m.choice = (m.choice + 1) % 27
		case "enter":
			return m.choose(ui.AlphabetLetters[m.choice : m.choice+1])
		}
	}
	return m, nil
}

func (m *Model) View() string {
	active := ui.AlphabetLetters[m.choice : m.choice+1]
	content := "Jump to letter\nType A–Z or #\n\n" + m.rail.View(active, m.theme) +
		"\n\n# Numbers / other\nEsc Cancel"
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(ui.Accent(m.theme)).Padding(1, 2).Render(content)
}

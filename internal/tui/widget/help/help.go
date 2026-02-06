package help

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/ui"
)

type HelpOverlayModel struct {
	keys   []ui.HelpKey
	theme  tint.Tint
	width  int
	height int
}

func NewHelpOverlayModel(keys []ui.HelpKey, theme tint.Tint) *HelpOverlayModel {
	return &HelpOverlayModel{
		keys:  keys,
		theme: theme,
	}
}

func (m *HelpOverlayModel) Init() tea.Cmd {
	return nil
}

func (m *HelpOverlayModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "?":
			return nil, nil // Signal to dismiss
		}
	}
	return m, nil
}

func (m *HelpOverlayModel) View() string {
	var sb strings.Builder

	accent := ui.Accent(m.theme)

	titleStyle := lipgloss.NewStyle().
		Foreground(accent).
		Bold(true).
		MarginBottom(1)

	keyStyle := lipgloss.NewStyle().
		Foreground(m.theme.BrightCyan()).
		Bold(true).
		Width(15)

	descStyle := lipgloss.NewStyle().
		Foreground(m.theme.White())

	sb.WriteString(titleStyle.Render(" COMMANDS "))
	sb.WriteString("\n\n")

	for _, k := range m.keys {
		row := lipgloss.JoinHorizontal(lipgloss.Top,
			keyStyle.Render(k.Key),
			descStyle.Render(k.Desc),
		)
		sb.WriteString(row + "\n")
	}

	sb.WriteString("\n" + lipgloss.NewStyle().Foreground(m.theme.BrightBlack()).Render(" Press esc, q, or ? to close "))

	content := sb.String()

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Background(lipgloss.Color("#1a1a1a")).
		Padding(1, 2).
		Width(max(m.width/2, 45)).
		Render(content)
}

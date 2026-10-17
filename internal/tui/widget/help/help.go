package help

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/ui"
)

type HelpOverlayModel struct {
	keys      []ui.HelpKey
	theme     tint.Tint
	width     int
	height    int
	offset    int
	maxOffset int
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

func (m *HelpOverlayModel) ActionRows() int { return 1 }

func (m *HelpOverlayModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseMsg:
		m.offset = max(0, min(m.maxOffset, m.offset+ui.WheelDelta(msg)*3))
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "?":
			return nil, nil // Signal to dismiss
		case "up", "k":
			m.offset = max(0, m.offset-1)
		case "down", "j":
			m.offset = min(m.maxOffset, m.offset+1)
		}
	}
	return m, nil
}

func (m *HelpOverlayModel) View() string {
	var sb strings.Builder
	boxWidth := max(20, min(m.width-2, 90))
	contentWidth := boxWidth - 4

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
		Foreground(m.theme.White()).Width(max(1, contentWidth-15))

	for _, k := range m.keys {
		row := lipgloss.JoinHorizontal(lipgloss.Top,
			keyStyle.Render(k.Key),
			descStyle.Render(k.Desc),
		)
		sb.WriteString(row + "\n")
	}

	rows := strings.TrimSuffix(sb.String(), "\n")
	vp := viewport.New(contentWidth, max(1, m.height-8))
	vp.SetContent(rows)
	vp.SetYOffset(m.offset)
	m.offset = vp.YOffset
	m.maxOffset = max(0, lipgloss.Height(rows)-vp.Height)
	content := lipgloss.JoinVertical(lipgloss.Left, titleStyle.Render(" COMMANDS "), vp.View(), "",
		lipgloss.NewStyle().Foreground(m.theme.BrightBlack()).Render(" [esc] Close | Wheel: scroll "))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Background(lipgloss.Color("#1a1a1a")).
		Padding(1, 2).
		Width(boxWidth).
		Render(content)
}

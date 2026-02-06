package resume

import (
	"fmt"
	"strings"

	"github.com/LukeHagar/plexgo/models/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/tui/player"
	"github.com/ygelfand/plexctl/internal/ui"
)

type ResumeDecisionMsg struct {
	Resume   bool
	Metadata *components.Metadata
	TctMode  bool
}

type ResumeOverlayModel struct {
	Metadata *components.Metadata
	TctMode  bool
	theme    tint.Tint
	choice   int // 0: Resume, 1: Start from beginning
	width    int
	height   int
}

func NewResumeOverlayModel(metadata *components.Metadata, tctMode bool, theme tint.Tint) *ResumeOverlayModel {
	return &ResumeOverlayModel{
		Metadata: metadata,
		TctMode:  tctMode,
		theme:    theme,
		choice:   0,
	}
}

func (m *ResumeOverlayModel) Init() tea.Cmd {
	return nil
}

func (m *ResumeOverlayModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			m.choice = 0
		case "down", "j":
			m.choice = 1
		case "enter":
			resume := m.choice == 0
			offset := int64(0)
			if resume && m.Metadata.ViewOffset != nil {
				offset = int64(*m.Metadata.ViewOffset)
			}
			return nil, player.PlayMedia(m.Metadata, false, m.TctMode, offset)
		case "esc":
			return nil, nil
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

func (m *ResumeOverlayModel) View() string {
	boxWidth := 40
	title := "Resume Playback?"
	if m.Metadata != nil {
		title = fmt.Sprintf("Resume %s?", m.Metadata.Title)
	}

	accent := ui.Accent(m.theme)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(accent).
		MarginBottom(1)

	optionStyle := lipgloss.NewStyle().PaddingLeft(2)
	selectedStyle := optionStyle.Copy().
		Foreground(accent).
		Bold(true)

	resumeText := "Resume"
	if m.Metadata != nil && m.Metadata.ViewOffset != nil {
		ms := *m.Metadata.ViewOffset
		s := ms / 1000
		h := s / 3600
		min := (s % 3600) / 60
		sec := s % 60
		if h > 0 {
			resumeText = fmt.Sprintf("Resume from %d:%02d:%02d", h, min, sec)
		} else {
			resumeText = fmt.Sprintf("Resume from %d:%02d", min, sec)
		}
	}

	var options []string
	if m.choice == 0 {
		options = append(options, "󰄬 "+selectedStyle.Render(resumeText))
		options = append(options, "  "+optionStyle.Render("Start from beginning"))
	} else {
		options = append(options, "  "+optionStyle.Render(resumeText))
		options = append(options, "󰄬 "+selectedStyle.Render("Start from beginning"))
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render(title),
		"",
		strings.Join(options, "\n"),
		"",
		lipgloss.NewStyle().Foreground(m.theme.BrightBlack()).Render("[↑/↓] Select  [enter] Confirm  [esc] Cancel"),
	)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ui.Accent(m.theme)).
		Padding(1, 2).
		Width(boxWidth).
		Render(content)
}

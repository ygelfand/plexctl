package userpicker

import (
	"fmt"
	"strings"

	"github.com/LukeHagar/plexgo/models/operations"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/ui"
)

type UserPickerOverlayModel struct {
	users       []operations.HomeUser
	cursor      int
	theme       tint.Tint
	enteringPin bool
	pinInput    string
	showInvalid bool
}

func NewUserPickerOverlayModel(users []operations.HomeUser, theme tint.Tint) *UserPickerOverlayModel {
	return &UserPickerOverlayModel{
		users: users,
		theme: theme,
	}
}

func (m *UserPickerOverlayModel) Init() tea.Cmd {
	return nil
}

func (m *UserPickerOverlayModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ui.InvalidPinMsg:
		m.showInvalid = true
		m.pinInput = ""
		return m, nil

	case tea.KeyMsg:
		if m.enteringPin {
			switch msg.String() {
			case "esc":
				m.enteringPin = false
				m.pinInput = ""
				m.showInvalid = false
				return m, nil
			case "enter":
				if len(m.pinInput) != 4 {
					return m, nil // Don't submit unless 4 digits
				}
				user := m.users[m.cursor]
				pin := m.pinInput
				// Don't clear state yet, wait for response
				return m, func() tea.Msg {
					return ui.SwitchUserMsg{User: user, Pin: pin}
				}
			case "backspace":
				if len(m.pinInput) > 0 {
					m.pinInput = m.pinInput[:len(m.pinInput)-1]
				}
			default:
				// Only allow 4 digits
				if len(m.pinInput) < 4 && strings.Contains("0123456789", msg.String()) {
					m.pinInput += msg.String()
					m.showInvalid = false // Clear error when typing
				}
			}
			return m, nil
		}

		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.users)-1 {
				m.cursor++
			}
		case "enter":
			user := m.users[m.cursor]
			if user.Protected {
				m.enteringPin = true
				m.pinInput = ""
				m.showInvalid = false
				return m, nil
			}
			return m, func() tea.Msg {
				return ui.SwitchUserMsg{User: user}
			}
		case "esc", "q":
			// Return nil model to indicate we want to pop the overlay
			return nil, nil
		}
	}
	return m, nil
}

func (m *UserPickerOverlayModel) View() string {
	accent := ui.Accent(m.theme)
	titleStyle := ui.TitleStyle(m.theme).MarginBottom(1)

	if m.enteringPin {
		user := m.users[m.cursor]
		maskedPin := strings.Repeat("•", len(m.pinInput))
		if len(m.pinInput) == 0 {
			maskedPin = "Enter PIN"
		}

		status := lipgloss.NewStyle().Foreground(m.theme.BrightBlack()).Render("(4 digits)")
		if m.showInvalid {
			status = ui.ErrorStyle(m.theme).Render("Invalid PIN")
		}

		content := lipgloss.JoinVertical(lipgloss.Center,
			titleStyle.Render(fmt.Sprintf("Enter PIN for %s", user.Title)),
			status,
			lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(accent).
				Padding(1, 4).
				Render(maskedPin),
		)
		return lipgloss.NewStyle().Padding(1, 2).Render(content)
	}

	var items []string
	for i, user := range m.users {
		style := lipgloss.NewStyle().Padding(0, 1)
		prefix := "  "
		if i == m.cursor {
			style = style.Foreground(accent).Bold(true)
			prefix = "> "
		}

		role := "User"
		if user.Admin {
			role = "Admin"
		}

		lock := ""
		if user.Protected {
			lock = " 🔒"
		}

		item := fmt.Sprintf("%s%-20s (%s)%s", prefix, user.Title, role, lock)
		items = append(items, style.Render(item))
	}

	content := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("Who's watching?"),
		lipgloss.JoinVertical(lipgloss.Left, items...),
	)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Padding(1, 2).
		Render(content)
}

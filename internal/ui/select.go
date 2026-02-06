package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type item struct {
	title, desc string
	value       string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title + " " + i.desc }

type itemDelegate struct{}

func (d itemDelegate) Height() int                               { return 1 }
func (d itemDelegate) Spacing() int                              { return 0 }
func (d itemDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }
func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}

	theme := CurrentTheme()
	itemStyle := lipgloss.NewStyle().PaddingLeft(4)
	selectedItemStyle := lipgloss.NewStyle().PaddingLeft(2).Foreground(Accent(theme))

	str := fmt.Sprintf("%d. %s (%s)", index+1, i.title, i.desc)

	fn := itemStyle.Render
	if index == m.Index() {
		fn = func(s ...string) string {
			return selectedItemStyle.Render("> " + strings.Join(s, " "))
		}
	}

	fmt.Fprint(w, fn(str))
}

type model struct {
	list     list.Model
	choice   string
	quitting bool
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.list.SetWidth(msg.Width)
		return m, nil

	case tea.KeyMsg:
		switch keypress := msg.String(); keypress {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit

		case "enter":
			i, ok := m.list.SelectedItem().(item)
			if ok {
				m.choice = i.value
			}
			m.quitting = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.choice != "" || m.quitting {
		return ""
	}
	return "\n" + m.list.View()
}

// SelectOption presents a list of options to the user and returns the selected value
func SelectOption(title string, options []struct{ Title, Desc, Value string }) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("no options provided")
	}

	var items []list.Item
	for _, o := range options {
		items = append(items, item{title: o.Title, desc: o.Desc, value: o.Value})
	}

	theme := CurrentTheme()
	l := list.New(items, itemDelegate{}, 60, 10)
	l.Title = title
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.Styles.Title = TitleStyle(theme)

	m := model{list: l}

	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return "", err
	}

	res := finalModel.(model).choice
	if res == "" {
		return "", fmt.Errorf("no selection made")
	}

	return res, nil
}

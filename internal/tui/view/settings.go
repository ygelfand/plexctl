package view

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/config"
)

type themeItem struct {
	id, name string
	active   bool
}

func (i themeItem) Title() string {
	if i.active {
		return fmt.Sprintf("%s (Active)", i.name)
	}
	return i.name
}
func (i themeItem) Description() string { return "ID: " + i.id }
func (i themeItem) FilterValue() string { return i.name }

type SettingsTab struct {
	list   list.Model
	width  int
	height int
}

func NewSettingsTab() *SettingsTab {
	cfg := config.Get()
	tints := tint.DefaultTints()
	var items []list.Item
	for _, t := range tints {
		items = append(items, themeItem{
			id:     t.ID(),
			name:   t.ID(), // Use ID as name if Name() is missing
			active: t.ID() == cfg.Theme,
		})
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Theme Selection"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)

	return &SettingsTab{
		list: l,
	}
}

func (t *SettingsTab) Init() tea.Cmd {
	return nil
}

func (t *SettingsTab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width = msg.Width
		t.height = msg.Height
		t.list.SetSize(msg.Width, msg.Height)
	case tea.KeyMsg:
		if msg.String() == "enter" {
			selected := t.list.SelectedItem().(themeItem)
			return t, func() tea.Msg {
				return ThemeSelectedMsg{ID: selected.id}
			}
		}
	}

	t.list, cmd = t.list.Update(msg)
	return t, cmd
}

func (t *SettingsTab) View() string {
	return t.list.View()
}

type ThemeSelectedMsg struct {
	ID string
}

func (t *SettingsTab) SetTheme(theme tint.Tint) {
	// Re-build items to show active status correctly
	var items []list.Item
	tints := tint.DefaultTints()
	for _, tintObj := range tints {
		items = append(items, themeItem{
			id:     tintObj.ID(),
			name:   tintObj.ID(),
			active: tintObj.ID() == theme.ID(),
		})
	}
	t.list.SetItems(items)

	// Apply style
	d := list.NewDefaultDelegate()
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.Foreground(theme.BrightYellow()).BorderForeground(theme.BrightYellow())
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.Foreground(theme.BrightBlack()).BorderForeground(theme.BrightYellow())
	t.list.SetDelegate(d)
}

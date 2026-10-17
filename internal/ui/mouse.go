package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Rect uses terminal cells, relative to the model receiving the mouse event.
type Rect struct {
	X, Y, Width, Height int
}

func (r Rect) Local(m tea.MouseMsg) (tea.MouseMsg, bool) {
	if m.X < r.X || m.Y < r.Y || m.X >= r.X+r.Width || m.Y >= r.Y+r.Height {
		return m, false
	}
	m.X -= r.X
	m.Y -= r.Y
	return m, true
}

func LeftClick(m tea.MouseMsg) bool {
	return m.Action == tea.MouseActionPress && m.Button == tea.MouseButtonLeft
}

func RightClick(m tea.MouseMsg) bool {
	return m.Action == tea.MouseActionPress && m.Button == tea.MouseButtonRight
}

// WheelDelta ignores button releases and motion, which must never activate a control.
func WheelDelta(m tea.MouseMsg) int {
	if m.Action != tea.MouseActionPress {
		return 0
	}
	switch m.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelLeft:
		return -1
	case tea.MouseButtonWheelDown, tea.MouseButtonWheelRight:
		return 1
	}
	return 0
}

func KeyPress(key string) tea.KeyMsg {
	types := map[string]tea.KeyType{
		"enter": tea.KeyEnter, "esc": tea.KeyEsc, "up": tea.KeyUp, "down": tea.KeyDown,
		"left": tea.KeyLeft, "right": tea.KeyRight, "tab": tea.KeyTab,
		"home": tea.KeyHome, "end": tea.KeyEnd, "pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown,
		"ctrl+s": tea.KeyCtrlS, "ctrl+h": tea.KeyCtrlH, "ctrl+j": tea.KeyCtrlJ,
		"ctrl+k": tea.KeyCtrlK, "ctrl+l": tea.KeyCtrlL, "space": tea.KeySpace,
		"↑": tea.KeyUp, "↓": tea.KeyDown, "arrows": tea.KeyTab,
	}
	if t, ok := types[key]; ok {
		return tea.KeyMsg{Type: t}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

// LabelAt tests the actual rendered cells, including wide Unicode characters.
// It is useful for small menus whose text may wrap after a terminal resize.
func LabelAt(view string, m tea.MouseMsg, label string) bool {
	lines := strings.Split(view, "\n")
	if m.Y < 0 || m.Y >= len(lines) || label == "" {
		return false
	}
	line := ansi.Strip(lines[m.Y])
	before, _, ok := strings.Cut(line, label)
	if !ok {
		return false
	}
	x := lipgloss.Width(before)
	return m.X >= x && m.X < x+lipgloss.Width(label)
}

// ShortcutAt makes visible [key] action bars clickable. Callers restrict this
// to their action bar, so media titles and descriptions cannot become actions.
func ShortcutAt(bar string, m tea.MouseMsg) string {
	if !LeftClick(m) {
		return ""
	}
	lines := strings.Split(bar, "\n")
	if m.Y < 0 || m.Y >= len(lines) {
		return ""
	}
	line := ansi.Strip(lines[m.Y])
	x := 0
	for part := range strings.SplitSeq(line, "|") {
		width := lipgloss.Width(part)
		if m.X >= x && m.X < x+width {
			start := strings.Index(part, "[")
			end := strings.Index(part, "]")
			if start >= 0 && end > start {
				key, _, _ := strings.Cut(part[start+1:end], "/")
				// The slash key is itself a valid action.
				if part[start+1:end] == "/" {
					key = "/"
				}
				return key
			}
			return ""
		}
		x += width + 1
	}
	return ""
}

// ListMouse handles lists using Bubbles' default two-line delegate. Coordinates
// are relative to the list, excluding its parent's border and padding.
// It returns true only when a visible item was clicked, ready for activation.
func ListMouse(m *list.Model, mouse tea.MouseMsg) bool {
	if _, inside := (Rect{Width: m.Width(), Height: m.Height()}).Local(mouse); !inside {
		return false
	}
	if delta := WheelDelta(mouse); delta != 0 {
		key := "down"
		if delta < 0 {
			key = "up"
		}
		*m, _ = m.Update(KeyPress(key))
		return false
	}
	if !LeftClick(mouse) || len(m.VisibleItems()) == 0 {
		return false
	}
	headerHeight := 0
	if m.ShowTitle() || (m.ShowFilter() && m.FilteringEnabled()) {
		title := m.Styles.Title.Render(m.Title) + "  "
		if m.FilterState() == list.Filtering {
			title = m.FilterInput.View()
		}
		headerHeight = lipgloss.Height(m.Styles.TitleBar.Render(title))
	}
	// All TUI lists hide the status bar; keep unsupported layouts inert.
	if m.ShowStatusBar() {
		return false
	}
	y := mouse.Y - headerHeight
	if y < 0 || y%3 >= 2 {
		return false
	}
	start, end := m.Paginator.GetSliceBounds(len(m.VisibleItems()))
	i := start + y/3
	if i >= end {
		return false
	}
	m.Select(i)
	return true
}

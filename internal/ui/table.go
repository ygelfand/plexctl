package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	tint "github.com/lrstanley/bubbletint"
)

// NewTable creates a new bubbles/table with standard initial settings
func NewTable(columns []table.Column, theme tint.Tint) Table {
	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
	)

	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(theme.BrightBlack()).
		BorderBottom(true).
		Bold(false)
	s.Selected = s.Selected.
		Foreground(theme.BrightWhite()).
		Background(Accent(theme)).
		Bold(false)
	t.SetStyles(s)

	return Table{Model: t}
}

// Table keeps an explicit visible row range for accurate mouse selection after
// keyboard scrolling, pagination, appended rows, and terminal resizes.
type Table struct {
	table.Model
	first int
	left  int
}

func (t *Table) syncScroll() {
	if t.Cursor() >= len(t.Rows()) {
		t.SetCursor(max(0, len(t.Rows())-1))
	}
	height := max(1, t.Height())
	t.first = min(t.first, max(0, len(t.Rows())-height))
	if t.Cursor() < t.first {
		t.first = max(0, t.Cursor())
	} else if t.Cursor() >= t.first+height {
		t.first = t.Cursor() - height + 1
	}
	t.left = max(0, min(t.left, t.contentWidth()-t.Width()))
}

func (t Table) contentWidth() int {
	width := 0
	for _, column := range t.Columns() {
		if column.Width > 0 {
			// NewTable's header and cell styles add one space on each side.
			width += column.Width + 2
		}
	}
	return width
}

func (t *Table) SetHeight(height int) { t.Model.SetHeight(max(3, height)) }

func (t *Table) SetWidth(width int) { t.Model.SetWidth(max(1, width)) }

func (t *Table) View() string {
	t.syncScroll()
	visible := t.Model
	end := min(len(t.Rows()), t.first+max(1, t.Height()))
	visible.SetRows(t.Rows()[t.first:end])
	visible.SetCursor(0)
	visible.GotoTop()
	visible.SetCursor(t.Cursor() - t.first)
	visible.SetWidth(max(t.Width(), t.contentWidth()))
	lines := strings.Split(visible.View(), "\n")
	for i, line := range lines {
		lines[i] = ansi.Cut(line, t.left, t.left+t.Width())
	}
	return strings.Join(lines, "\n")
}

// Click selects a visible row and reports whether it can be activated.
func (t *Table) Click(mouse tea.MouseMsg) bool {
	if !LeftClick(mouse) || mouse.X < 0 || mouse.X >= t.Width() {
		return false
	}
	t.syncScroll()
	// Standard tables have a one-line heading and its bottom border.
	row := mouse.Y - 2
	if row < 0 || row >= t.Height() || t.first+row >= len(t.Rows()) {
		return false
	}
	t.SetCursor(t.first + row)
	return true
}

func (t Table) Update(msg tea.Msg) (Table, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && t.Focused() {
		switch key.String() {
		case "h", "left":
			t.left -= 3
		case "l", "right":
			t.left += 3
		}
	}
	if mouse, ok := msg.(tea.MouseMsg); ok {
		if _, inside := (Rect{Width: t.Width(), Height: t.Height() + 2}).Local(mouse); inside {
			switch delta := WheelDelta(mouse); {
			case delta != 0 && (mouse.Shift || mouse.Button == tea.MouseButtonWheelLeft || mouse.Button == tea.MouseButtonWheelRight):
				t.left += delta * 3
			case delta < 0:
				t.MoveUp(3)
			case delta > 0:
				t.MoveDown(3)
			default:
				t.Click(mouse)
			}
		}
		t.syncScroll()
		return t, nil
	}
	var cmd tea.Cmd
	t.Model, cmd = t.Model.Update(msg)
	t.syncScroll()
	return t, cmd
}

// UpdateTableTheme refreshes a table's styles based on the current layout theme
func UpdateTableTheme(t *Table, width, height int) {
	layout := GetLayout()
	theme := layout.Theme()

	rows := t.Rows()
	cursor := t.Cursor()
	cols := t.Columns()
	first, left := t.first, t.left

	*t = NewTable(cols, theme)
	t.SetRows(rows)
	t.SetCursor(cursor)
	t.SetWidth(width)
	t.SetHeight(GetTableHeight(height))
	t.first, t.left = first, left
	t.syncScroll()
}

// GetTableHeight returns the appropriate table height based on available screen space
func GetTableHeight(totalHeight int) int {
	return max(3, min(totalHeight-3, GetLayout().ContentHeight()))
}

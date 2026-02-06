package ui

import (
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
)

// NewTable creates a new bubbles/table with standard initial settings
func NewTable(columns []table.Column, theme tint.Tint) table.Model {
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

	return t
}

// UpdateTableTheme refreshes a table's styles based on the current layout theme
func UpdateTableTheme(t *table.Model, width, height int) {
	layout := GetLayout()
	theme := layout.Theme()

	rows := t.Rows()
	cursor := t.Cursor()
	cols := t.Columns()

	*t = NewTable(cols, theme)
	t.SetRows(rows)
	t.SetCursor(cursor)
	t.SetWidth(width)
	t.SetHeight(GetTableHeight(height))
}

// GetTableHeight returns the appropriate table height based on available screen space
func GetTableHeight(totalHeight int) int {
	return max(totalHeight-5, 1)
}

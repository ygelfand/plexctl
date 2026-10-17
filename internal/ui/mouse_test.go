package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func click(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
}

func TestTableMouseAfterScrollResizeAndReplacement(t *testing.T) {
	m := NewTable([]table.Column{{Title: "TITLE", Width: 20}}, PlexctlTheme)
	rows := make([]table.Row, 30)
	for i := range rows {
		rows[i] = table.Row{fmt.Sprintf("Item %02d", i)}
	}
	m.SetRows(rows)
	m.SetWidth(25)
	m.SetHeight(8)
	for range 16 {
		m, _ = m.Update(KeyPress("down"))
	}
	for _, height := range []int{8, 5, 12} {
		m.SetHeight(height)
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		want := strings.TrimSpace(lines[3])
		if !m.Click(click(4, 3)) || m.SelectedRow()[0] != want {
			t.Fatalf("height %d: clicked visible %q, selected %v", height, want, m.SelectedRow())
		}
		if m.Click(click(4, 1)) || m.Click(click(30, 3)) || m.Click(click(4, 90)) {
			t.Fatal("header or off-table click activated a row")
		}
	}
	m.SetRows(nil)
	if m.Click(click(4, 3)) {
		t.Fatal("empty table activated a row")
	}
	_ = m.View() // Replacing a scrolled table with no rows must not panic.
}

type mouseListItem string

func (i mouseListItem) Title() string       { return string(i) }
func (i mouseListItem) Description() string { return "Description" }
func (i mouseListItem) FilterValue() string { return string(i) }

func TestListMouseUsesVisiblePage(t *testing.T) {
	items := make([]list.Item, 20)
	for i := range items {
		items[i] = mouseListItem(fmt.Sprintf("Option %02d", i))
	}
	m := list.New(items, list.NewDefaultDelegate(), 45, 16)
	m.Title = "Choose an option"
	m.SetShowStatusBar(false)
	m.SetFilteringEnabled(false)
	for _, index := range []int{0, 9, 18} {
		m.Select(index)
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		for y, line := range lines {
			var want int
			if start := strings.Index(line, "Option "); start >= 0 {
				if _, err := fmt.Sscanf(line[start:], "Option %d", &want); err != nil {
					t.Fatal(err)
				}
				if !ListMouse(&m, click(10, y)) || m.Index() != want {
					t.Fatalf("page click row %d: got %d, want %d\n%s", y, m.Index(), want, m.View())
				}
			}
		}
	}
	if ListMouse(&m, click(10, 0)) {
		t.Fatal("title click activated an item")
	}
}

func TestMouseIgnoresReleaseMotionAndMargins(t *testing.T) {
	for _, action := range []tea.MouseAction{tea.MouseActionRelease, tea.MouseActionMotion} {
		m := click(2, 3)
		m.Action = action
		if LeftClick(m) || ShortcutAt("[v] Video", m) != "" {
			t.Fatal("release or drag activated a control")
		}
	}
	if got := ShortcutAt("[p] Play | [v] Video | [/] Search", click(26, 0)); got != "/" {
		t.Fatalf("slash action = %q", got)
	}
	view := "📺 [v] Video"
	if !LabelAt(view, click(7, 0), "Video") || LabelAt(view, click(2, 0), "Video") {
		t.Fatal("wide character label hit test is incorrect")
	}
}

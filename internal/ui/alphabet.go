package ui

import (
	"strings"
	"unicode"

	"github.com/LukeHagar/plexgo/models/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tint "github.com/lrstanley/bubbletint"
	"golang.org/x/text/unicode/norm"
)

const AlphabetLetters = "#ABCDEFGHIJKLMNOPQRSTUVWXYZ"

type JumpToLetterMsg struct{ Letter string }

type LetterJumper interface {
	CanJumpLetters() bool
	JumpToLetter(string) tea.Cmd
}

type AlphabetNavigator interface {
	LetterJumper
	AlphabetRail() *AlphabetRail
}

// AlphabetRail folds into additional columns in short panes. The same geometry
// drives rendering and hit testing, including after the music player resizes it.
type AlphabetRail struct {
	Rect
	Focused bool
	cursor  int
}

func NewAlphabetRail(right, top, height int) AlphabetRail {
	if height < 5 || right < 12 {
		return AlphabetRail{}
	}
	rows := min(len(AlphabetLetters), height)
	width := ((len(AlphabetLetters) + rows - 1) / rows) * 3
	if width+8 > right {
		return AlphabetRail{}
	}
	return AlphabetRail{Rect: Rect{X: right - width, Y: top, Width: width, Height: rows}}
}

// SetBounds retains keyboard selection when the rail folds after a resize.
func (r *AlphabetRail) SetBounds(right, top, height int) {
	r.Rect = NewAlphabetRail(right, top, height).Rect
}

func (r *AlphabetRail) Focus(letter string) {
	r.Focused = true
	r.cursor = max(0, strings.Index(AlphabetLetters, letter))
}

func (r *AlphabetRail) MoveCursor(delta int) {
	r.cursor = max(0, min(len(AlphabetLetters)-1, r.cursor+delta))
}

func (r AlphabetRail) SelectedLetter() string {
	return AlphabetLetters[r.cursor : r.cursor+1]
}

func (r AlphabetRail) LetterAt(mouse tea.MouseMsg) string {
	local, inside := r.Local(mouse)
	if !inside || !LeftClick(mouse) {
		return ""
	}
	i := (local.X/3)*r.Height + local.Y
	if i >= len(AlphabetLetters) {
		return ""
	}
	return AlphabetLetters[i : i+1]
}

func (r AlphabetRail) View(active string, theme tint.Tint) string {
	if r.Width == 0 {
		return ""
	}
	rows := make([]string, r.Height)
	for y := range rows {
		for x := range r.Width / 3 {
			i := x*r.Height + y
			cell := "   "
			if i < len(AlphabetLetters) {
				letter := AlphabetLetters[i : i+1]
				label := " " + letter + " "
				style := lipgloss.NewStyle().Foreground(theme.BrightBlack())
				switch {
				case r.Focused && i == r.cursor:
					label = "[" + letter + "]"
					style = style.Foreground(theme.Black()).Background(Accent(theme)).Bold(true)
				case r.Focused && letter == active:
					style = style.Foreground(Accent(theme)).Bold(true)
				case !r.Focused && letter == active:
					style = style.Foreground(theme.Black()).Background(Accent(theme)).Bold(true)
				}
				cell = style.Render(label)
			}
			rows[y] += cell
		}
	}
	return strings.Join(rows, "\n")
}

// AlphabetTitle respects Plex's sort title (including its article handling).
// Latin accents share their base letter; numbers and other alphabets use #.
func AlphabetTitle(meta components.Metadata) string {
	title := meta.Title
	if meta.TitleSort != nil && *meta.TitleSort != "" {
		title = *meta.TitleSort
	}
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		return unicode.ToUpper(r)
	}, norm.NFD.String(strings.TrimSpace(title)))
}

func AlphabetLetter(meta components.Metadata) string {
	title := AlphabetTitle(meta)
	if len(title) > 0 && title[0] >= 'A' && title[0] <= 'Z' {
		return title[:1]
	}
	return "#"
}

func AlphabetIndex(items []components.Metadata, letter string) int {
	for i, item := range items {
		if AlphabetLetter(item) == letter {
			return i
		}
	}
	return -1
}

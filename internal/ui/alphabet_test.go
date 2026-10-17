package ui

import (
	"strings"
	"testing"

	"github.com/LukeHagar/plexgo/models/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestAlphabetRailMouseMatchesRenderedLettersAfterResize(t *testing.T) {
	for _, height := range []int{40, 27, 26, 14, 9, 6, 5} {
		rail := NewAlphabetRail(50, 4, height)
		seen := ""
		for y, line := range strings.Split(ansi.Strip(rail.View("M", PlexctlTheme)), "\n") {
			for x, letter := range line {
				if letter == ' ' {
					continue
				}
				mouse := tea.MouseMsg{X: rail.X + x, Y: rail.Y + y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
				if got := rail.LetterAt(mouse); got != string(letter) {
					t.Fatalf("height %d: clicked %c, got %q", height, letter, got)
				}
				mouse.Action = tea.MouseActionRelease
				if rail.LetterAt(mouse) != "" {
					t.Fatal("release activated a letter")
				}
				seen += string(letter)
			}
		}
		if len(seen) != 27 {
			t.Fatalf("height %d: only %d letters visible", height, len(seen))
		}
	}
}

func TestAlphabetRespectsSortTitleAccentsAndNumbers(t *testing.T) {
	for _, test := range []struct {
		meta components.Metadata
		want string
	}{
		{components.Metadata{Title: "The Matrix", TitleSort: Ptr("Matrix")}, "M"},
		{components.Metadata{Title: " Émilie"}, "E"},
		{components.Metadata{Title: "123"}, "#"},
		{components.Metadata{Title: "東京"}, "#"},
		{components.Metadata{}, "#"},
	} {
		if got := AlphabetLetter(test.meta); got != test.want {
			t.Fatalf("%q: got %q, want %q", test.meta.Title, got, test.want)
		}
	}
}

func TestAlphabetKeyboardSelectionSurvivesFolding(t *testing.T) {
	rail := NewAlphabetRail(50, 4, 40)
	rail.Focus("L")
	rail.MoveCursor(1)
	for _, height := range []int{40, 14, 9, 6, 5, 40} {
		rail.SetBounds(50, 4, height)
		if !rail.Focused || rail.SelectedLetter() != "M" {
			t.Fatalf("height %d lost keyboard selection", height)
		}
		view := ansi.Strip(rail.View("A", PlexctlTheme))
		if strings.Count(view, "[") != 1 || !strings.Contains(view, "[M]") {
			t.Fatalf("height %d did not highlight the cursor independently of the active title", height)
		}
		for y, line := range strings.Split(view, "\n") {
			if x := strings.Index(line, "[M]"); x >= 0 {
				for offset := range 3 {
					mouse := tea.MouseMsg{X: rail.X + x + offset, Y: rail.Y + y,
						Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
					if got := rail.LetterAt(mouse); got != "M" {
						t.Fatalf("height %d: clicking focused letter cell returned %q", height, got)
					}
				}
			}
		}
	}
	rail.Focused = false
	if strings.Contains(rail.View("A", PlexctlTheme), "[M]") {
		t.Fatal("unfocused rail kept its keyboard cursor")
	}
}

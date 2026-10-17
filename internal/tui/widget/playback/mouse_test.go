package playback

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/ui"
)

func TestClickSelectsRendererAtDifferentWidths(t *testing.T) {
	for _, width := range []int{50, 100} {
		m := New("42", "A movie with a long title", ui.PlexctlTheme)
		m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		clicked := false
		for y, line := range strings.Split(ansi.Strip(m.View()), "\n") {
			x := strings.Index(line, "Kitty graphics")
			if x < 0 {
				continue
			}
			mouse := tea.MouseMsg{X: x + 2, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease}
			if next, cmd := m.Update(mouse); next == nil || cmd != nil {
				t.Fatal("release activated renderer")
			}
			mouse.Action = tea.MouseActionPress
			next, cmd := m.Update(mouse)
			if next != nil || cmd == nil {
				t.Fatal("renderer click did not close menu and request playback")
			}
			request := cmd().(ui.RequestPlayMsg)
			if request.RatingKey != "42" || request.Output != config.VideoKitty {
				t.Fatalf("wrong playback request: %#v", request)
			}
			clicked = true
			break
		}
		if !clicked {
			t.Fatal("Kitty renderer was not visible")
		}
	}
}

package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ygelfand/plexctl/internal/ui"
)

type mouseOverlay struct{ last tea.Msg }

func (m *mouseOverlay) Init() tea.Cmd                           { return nil }
func (m *mouseOverlay) View() string                            { return "1234567890\n1234567890\n1234567890\n1234567890" }
func (m *mouseOverlay) Update(msg tea.Msg) (tea.Model, tea.Cmd) { m.last = msg; return m, nil }

func TestOverlayCapturesAndTranslatesMouse(t *testing.T) {
	n := NewNavigator(ui.PlexctlTheme)
	n.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	o := &mouseOverlay{}
	n.Push(o)
	o.last = nil
	for _, mouse := range []tea.MouseMsg{
		{X: 1, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress},
		{X: 20, Y: 12, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress},
	} {
		if _, captured := n.Update(mouse); !captured || o.last != nil {
			t.Fatal("background mouse event escaped modal bounds")
		}
	}
	n.Update(tea.MouseMsg{X: 37, Y: 11, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if mouse, ok := o.last.(tea.MouseMsg); !ok || mouse.X != 2 || mouse.Y != 1 {
		t.Fatalf("local mouse = %#v", o.last)
	}
	n.Update(tea.MouseMsg{X: 1, Y: 1, Button: tea.MouseButtonRight, Action: tea.MouseActionPress})
	if key, ok := o.last.(tea.KeyMsg); !ok || key.String() != "esc" {
		t.Fatalf("right click = %#v", o.last)
	}
}

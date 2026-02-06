package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	tint "github.com/lrstanley/bubbletint"
	"github.com/ygelfand/plexctl/internal/ui"
)

// Overlay represents a model that is rendered on top of the main view
type Overlay interface {
	tea.Model
}

type Navigator struct {
	overlays []Overlay
	theme    tint.Tint
	width    int
	height   int
}

func NewNavigator(theme tint.Tint) *Navigator {
	return &Navigator{
		theme: theme,
	}
}

func (n *Navigator) Push(o Overlay) tea.Cmd {
	n.overlays = append(n.overlays, o)
	var cmds []tea.Cmd
	cmds = append(cmds, o.Init())
	// Propagate dimensions if we have them
	if n.width > 0 && n.height > 0 {
		_, cmd := o.Update(tea.WindowSizeMsg{Width: n.width, Height: n.height})
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
}

func (n *Navigator) Pop() {
	if len(n.overlays) > 0 {
		n.overlays = n.overlays[:len(n.overlays)-1]
	}
}

func (n *Navigator) ActiveOverlay() Overlay {
	if len(n.overlays) == 0 {
		return nil
	}
	return n.overlays[len(n.overlays)-1]
}

func (n *Navigator) Update(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		n.width = msg.Width
		n.height = msg.Height
	}

	overlay := n.ActiveOverlay()
	if overlay == nil {
		return nil, false
	}

	newModel, cmd := overlay.Update(msg)
	if newModel == nil {
		n.Pop()
		return cmd, true
	}
	n.overlays[len(n.overlays)-1] = newModel.(Overlay)

	// Key and Mouse events are always captured by overlays
	switch msg.(type) {
	case tea.KeyMsg, tea.MouseMsg:
		return cmd, true
	}

	return cmd, false
}

func (n *Navigator) Render(base string) string {
	if len(n.overlays) == 0 {
		return base
	}

	for _, o := range n.overlays {
		base = ui.Overlay(base, o.View(), n.width, n.height)
	}
	return base
}

func (n *Navigator) SetTheme(theme tint.Tint) {
	n.theme = theme
}

// KeyBindingHelper is a helper to check if a key matches a binding
func IsKey(msg tea.KeyMsg, binding key.Binding) bool {
	return key.Matches(msg, binding)
}

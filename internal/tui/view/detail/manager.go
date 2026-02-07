package detail

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ygelfand/plexctl/internal/ui"
)

type DetailManager struct {
	View tea.Model
}

func (m *DetailManager) Active() bool {
	return m.View != nil
}

func (m *DetailManager) Set(view tea.Model) tea.Cmd {
	m.View = view
	if m.View != nil {
		return m.View.Init()
	}
	return nil
}

func (m *DetailManager) Update(msg tea.Msg) (tea.Cmd, bool) {
	if m.View == nil {
		return nil, false
	}

	if _, ok := msg.(BackMsg); ok {
		if rc, ok := m.View.(ui.RootChecker); !ok || rc.IsAtRoot() {
			m.View = nil
			return nil, true
		}
	}

	if _, ok := msg.(BackToLibraryMsg); ok {
		m.View = nil
		return nil, true
	}

	var cmd tea.Cmd
	m.View, cmd = m.View.Update(msg)

	if _, ok := msg.(ui.MediaPageMsg); ok {
		return cmd, false
	}
	if _, ok := msg.(ui.JumpToDetailMsg); ok {
		return cmd, false
	}

	return cmd, true
}

func (m *DetailManager) ViewContent() string {
	if m.View == nil {
		return ""
	}
	return m.View.View()
}

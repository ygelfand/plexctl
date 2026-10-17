package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ygelfand/plexctl/internal/ui"
)

type paneFocus uint8

const (
	paneContent paneFocus = iota
	paneSidebar
	panePlayer
	paneAlphabet
)

func (c *Controller) focusPane(pane paneFocus) {
	if c.focusedAlphabet != nil {
		c.focusedAlphabet.Focused = false
		c.focusedAlphabet = nil
	}
	if pane == panePlayer && !c.playerStatus.Active() {
		pane = paneContent
	}
	if pane == paneAlphabet {
		if rail := c.alphabetRail(); rail != nil && rail.Width > 0 {
			letter := "#"
			if provider, ok := c.tabManager.ActiveModel().(ui.PlayableProvider); ok {
				if meta := provider.GetSelectedMetadata(); meta != nil {
					letter = ui.AlphabetLetter(*meta)
				}
			}
			rail.Focus(letter)
			c.focusedAlphabet = rail
		} else {
			pane = paneContent
		}
	}
	c.focus = pane
	c.tabManager.navbar.SetFocused(pane == paneSidebar)
}

func (c *Controller) alphabetRail() *ui.AlphabetRail {
	if target, ok := c.tabManager.ActiveModel().(ui.AlphabetNavigator); ok && target.CanJumpLetters() {
		return target.AlphabetRail()
	}
	return nil
}

func (c *Controller) validateAlphabetFocus() {
	if c.focus == paneAlphabet {
		if rail := c.alphabetRail(); rail == nil || rail.Width == 0 || rail != c.focusedAlphabet {
			c.focusPane(paneContent)
		}
	}
}

func (c *Controller) paneBorder(pane paneFocus) lipgloss.TerminalColor {
	if c.focus == pane {
		return ui.Accent(c.theme)
	}
	return c.theme.BrightBlack()
}

// Pane keys run after modal input handling, so text fields keep their editing
// keys. Directional moves stop at the edge, including when a key is held down.
func (c *Controller) paneKey(key string) (tea.Cmd, bool) {
	c.validateAlphabetFocus()
	switch key {
	case "ctrl+h":
		if c.focus == paneAlphabet {
			c.focusPane(paneContent)
		} else {
			c.focusPane(paneSidebar)
		}
	case "ctrl+l":
		switch c.focus {
		case paneSidebar:
			c.focusPane(paneContent)
		case paneContent:
			c.focusPane(paneAlphabet)
		}
	case "ctrl+j":
		if (c.focus == paneContent || c.focus == paneAlphabet) && c.playerStatus.Active() {
			c.focusPane(panePlayer)
		}
	case "ctrl+k":
		if c.focus == panePlayer {
			c.focusPane(paneContent)
		}
	default:
		return c.focusedKey(key)
	}
	return nil, true
}

func (c *Controller) focusedKey(key string) (tea.Cmd, bool) {
	if c.focus == paneAlphabet {
		rail := c.focusedAlphabet
		switch key {
		case "j", "down":
			rail.MoveCursor(1)
		case "k", "up":
			rail.MoveCursor(-1)
		case "home":
			rail.MoveCursor(-len(ui.AlphabetLetters))
		case "end":
			rail.MoveCursor(len(ui.AlphabetLetters))
		case "enter":
			letter := rail.SelectedLetter()
			c.focusPane(paneContent)
			if target, ok := c.tabManager.ActiveModel().(ui.LetterJumper); ok {
				return target.JumpToLetter(letter), true
			}
		case "esc", "h", "left":
			c.focusPane(paneContent)
		default:
			return nil, false
		}
		return nil, true
	}
	if c.focus == paneSidebar {
		nav := c.tabManager.navbar
		switch key {
		case "j", "down":
			nav.MoveCursor(1)
		case "k", "up":
			nav.MoveCursor(-1)
		case "home":
			nav.MoveCursor(-len(c.tabManager.tabModels))
		case "end":
			nav.MoveCursor(len(c.tabManager.tabModels))
		case "pgup":
			nav.MoveCursor(-max(1, nav.Height-10))
		case "pgdown":
			nav.MoveCursor(max(1, nav.Height-10))
		case "enter", "l", "right":
			cmd := c.tabManager.SetActive(nav.Cursor())
			c.focusPane(paneContent)
			return cmd, true
		case "esc":
			c.focusPane(paneContent)
		case "h", "left": // Already at the left edge.
		default:
			return nil, false
		}
		return nil, true
	}
	if c.focus == panePlayer {
		switch key {
		case "enter":
			return c.player.TogglePause(), true
		case "h", "left":
			return c.player.Seek(-10), true
		case "l", "right":
			return c.player.Seek(10), true
		case "k", "up":
			return c.player.Volume(5), true
		case "j", "down":
			return c.player.Volume(-5), true
		case "home":
			return c.player.SeekPercent(0), true
		case "end":
			return c.player.SeekPercent(100), true
		case "esc":
			c.focusPane(paneContent)
			return nil, true
		}
	}
	return nil, false
}

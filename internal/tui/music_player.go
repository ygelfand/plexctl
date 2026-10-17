package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/tui/widget/visualizer"
	"github.com/ygelfand/plexctl/internal/ui"
)

func (c *Controller) visualHeight(height int) int {
	if config.Get().MusicVisualizer == "off" || height < 20 {
		return 0
	}
	if config.Get().MusicVisualizer == "waveform" {
		return max(2, min(8, (height-16)/3))
	}
	return max(2, min(6, (height-16)/4))
}

func (c *Controller) playerHeight(height int) int {
	if !c.playerStatus.AudioOnly {
		return 4
	}
	if visualHeight := c.visualHeight(height); visualHeight > 0 {
		return 6 + visualHeight
	}
	return 5
}

func (c *Controller) refreshPlayer() tea.Cmd {
	previous := c.playerStatus
	c.playerStatus = c.player.Status()
	if c.focus == panePlayer && !c.playerStatus.Active() {
		c.focusPane(paneContent)
	}
	if previous.Key != c.playerStatus.Key {
		c.visualizer = visualizer.Model{}
	}
	now := time.Now()
	elapsed := now.Sub(c.visualTick)
	c.visualTick = now
	c.visualizer.Step(c.playerStatus.Spectrum, c.playerStatus.Paused(), elapsed)
	c.visualizer.StepWaveform(c.playerStatus.Waveform, c.playerStatus.Paused(), elapsed)
	if previous.Active() != c.playerStatus.Active() || previous.AudioOnly != c.playerStatus.AudioOnly {
		if previous.Overlay || c.playerStatus.Overlay {
			return tea.Batch(resizePlayer(), tea.ClearScreen)
		}
		return resizePlayer()
	}
	return nil
}

func resizePlayer() tea.Cmd {
	layout := ui.GetLayout()
	width, height := layout.TotalWidth(), layout.TotalHeight()
	return func() tea.Msg { return tea.WindowSizeMsg{Width: width, Height: height} }
}

func (c *Controller) musicControl(key string) tea.Cmd {
	switch key {
	case "[":
		return c.player.Seek(-10)
	case "]":
		return c.player.Seek(10)
	case "+", "=":
		return c.player.Volume(5)
	case "-":
		return c.player.Volume(-5)
	case "m":
		return c.player.ToggleMute()
	case "z":
		if c.playerStatus.AudioOnly {
			cfg := config.Get()
			switch cfg.MusicVisualizer {
			case "mirror":
				cfg.MusicVisualizer = "off"
			case "off":
				cfg.MusicVisualizer = "waveform"
			case "spectrum":
				cfg.MusicVisualizer = "mirror"
			default:
				cfg.MusicVisualizer = "spectrum"
			}
			_ = cfg.Save()
			return resizePlayer()
		}
	}
	return nil
}

func (c *Controller) musicControls() string {
	volume := fmt.Sprintf("%.0f%%", c.playerStatus.Volume)
	if c.playerStatus.Muted {
		volume = "MUTED"
	}
	if ui.GetLayout().InnerWidth() < 75 {
		return "[space] | [-] " + volume + " | [+] | [m] | [z] | [x]"
	}
	return "[space] Pause | [-] " + volume + " | [+] | [m] Mute | [z] Visuals | [x] Stop"
}

func (c *Controller) renderMusicPlayer() string {
	layout := ui.GetLayout()
	width := max(1, layout.InnerWidth())
	state := "PLAYING"
	if c.playerStatus.Paused() {
		state = "PAUSED"
	}
	state = lipgloss.NewStyle().Foreground(c.theme.BrightCyan()).Render(state)
	title := lipgloss.NewStyle().Bold(true).Foreground(c.theme.BrightYellow()).
		Render(ui.Ellipsis(c.playerStatus.Title, max(1, width-10)))
	title += strings.Repeat(" ", max(1, width-lipgloss.Width(title)-lipgloss.Width(state))) + state
	lines := []string{ui.Ellipsis(title, width)}
	if height := c.visualHeight(layout.TotalHeight()); height > 0 {
		mode := config.Get().MusicVisualizer
		if mode != "mirror" && mode != "spectrum" {
			mode = "waveform"
		}
		lines = append(lines, c.visualizer.View(width, height, mode))
	}
	lines = append(lines, c.musicProgress(width), ui.Ellipsis(c.musicControls(), width))
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(c.paneBorder(panePlayer)).
		Padding(0, 1).Width(width + 2).Render(strings.Join(lines, "\n"))
}

func (c *Controller) musicProgress(width int) string {
	clock := fmt.Sprintf("%s / %s", ui.FormatDuration(int(c.playerStatus.Time*1000)),
		ui.FormatDuration(int(c.playerStatus.Duration*1000)))
	barWidth := max(1, width-len(clock)-2)
	progress := 0.0
	if c.playerStatus.Duration > 0 {
		progress = max(0, min(1, c.playerStatus.Time/c.playerStatus.Duration))
	}
	filled := min(barWidth-1, int(progress*float64(barWidth)))
	bar := lipgloss.NewStyle().Foreground(ui.Accent(c.theme)).Render(strings.Repeat("━", filled)+"●") +
		lipgloss.NewStyle().Foreground(c.theme.BrightBlack()).Render(strings.Repeat("─", barWidth-filled-1))
	return ui.Ellipsis(bar+"  "+clock, width)
}

func (c *Controller) playerMouse(mouse tea.MouseMsg, top int) tea.Cmd {
	if delta := ui.WheelDelta(mouse); delta != 0 {
		if mouse.Shift {
			return c.player.Volume(float64(-delta * 5))
		}
		return c.player.Seek(float64(delta * 10))
	}
	if !ui.LeftClick(mouse) {
		return nil
	}
	if !c.playerStatus.AudioOnly {
		return c.player.TogglePause()
	}
	view := c.renderMusicPlayer()
	local := mouse
	local.X -= ui.SidebarTotalWidth
	local.Y -= top
	progressRow := lipgloss.Height(view) - 3
	if local.Y == progressRow {
		clock := fmt.Sprintf("%s / %s", ui.FormatDuration(int(c.playerStatus.Time*1000)),
			ui.FormatDuration(int(c.playerStatus.Duration*1000)))
		barWidth := max(1, ui.GetLayout().InnerWidth()-len(clock)-2)
		return c.player.SeekPercent(float64(local.X-2) / float64(max(1, barWidth-1)) * 100)
	}
	if local.Y == progressRow+1 {
		if key := ui.ShortcutAt(view, local); key != "" {
			switch key {
			case "space":
				return c.player.TogglePause()
			case "x":
				return c.player.StopPlayback()
			default:
				return c.musicControl(key)
			}
		}
	}
	return c.player.TogglePause()
}

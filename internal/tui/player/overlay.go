package player

import (
	"fmt"
	"image"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/termgraphics"
	"github.com/ygelfand/plexctl/internal/tui/artwork"
	"github.com/ygelfand/plexctl/internal/ui"
)

func isOverlayOutput(output config.VideoOutput) bool {
	return output != config.VideoWindow && output != config.AudioTerminal
}

func overlayBounds() image.Rectangle {
	layout := ui.GetLayout()
	// Reserve the compact player bar even before its first status update.
	height := min(layout.ContentHeight(), max(0, layout.TotalHeight()-7))
	return image.Rect(ui.SidebarTotalWidth+2, 1, ui.SidebarTotalWidth+2+layout.InnerWidth(), 1+height)
}

func overlayArgs(output config.VideoOutput, widthLimit int) []string {
	rect := overlayBounds()
	cols, rows := max(1, rect.Dx()), max(2, rect.Dy())
	if output == config.VideoTCT {
		return []string{fmt.Sprintf("--vo-tct-width=%d", cols), fmt.Sprintf("--vo-tct-height=%d", rows)}
	}
	prefix := "--vo-" + string(output) + "-"
	caps := termgraphics.Current()
	cw, ch := caps.CellWidth, caps.CellHeight
	if cw <= 0 || ch <= 0 {
		cw, ch = 8, 16
	}
	args := []string{prefix + "alt-screen=no", prefix + "config-clear=no", prefix + "left=1", prefix + "top=1",
		fmt.Sprintf("%scols=%d", prefix, cols), fmt.Sprintf("%srows=%d", prefix, rows),
		fmt.Sprintf("%swidth=%d", prefix, cols*cw), fmt.Sprintf("%sheight=%d", prefix, rows*ch)}
	if output == config.VideoSixel {
		args = append(args, sixelSizeArgs(widthLimit, cols, rows, cw, ch)...)
	}
	if output == config.VideoKitty {
		args = append(args, prefix+"use-shm=no")
	}
	return args
}

func (pm *PlayerManager) configureOverlay(cmd *exec.Cmd, output config.VideoOutput) *termgraphics.OverlayWriter {
	w := termgraphics.NewOverlayWriter(artwork.VideoOutput(), overlayBounds, output == config.VideoKitty)
	cmd.Stdout = w
	cmd.Args = append(cmd.Args, "--vo="+string(output), "--terminal=yes", "--input-terminal=no",
		"--really-quiet", "--osc=no", "--profile=sw-fast")
	cmd.Args = append(cmd.Args, overlayArgs(output, config.Get().SixelVideoWidth)...)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "TMUX=") && !strings.HasPrefix(entry, "TMUX_PANE=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	return w
}

func (pm *PlayerManager) SetOverlayVisible(visible bool) {
	if overlay := pm.overlay.Load(); overlay != nil {
		if overlay.SetVisible(visible) && visible {
			pm.redrawOverlay.Store(true)
		}
	}
}

func (pm *PlayerManager) ResizeOverlay() tea.Cmd {
	return func() tea.Msg {
		pm.ioMu.Lock()
		defer pm.ioMu.Unlock()
		if pm.overlay.Load() == nil || pm.conn == nil {
			return nil
		}
		var graph string
		if err := pm.conn.get("lavfi-complex", &graph); err != nil {
			return err
		}
		if err := resizeVideoOutput(pm.output, overlayArgs(pm.output, config.Get().SixelVideoWidth), graph, pm.conn.call); err != nil {
			return err
		}
		pm.redrawOverlay.Store(true)
		return nil
	}
}

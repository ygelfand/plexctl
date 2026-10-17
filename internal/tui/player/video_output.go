package player

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/termgraphics"
)

func terminalOutput(ctx context.Context, preference config.VideoOutput, getenv func(string) string) (string, config.VideoOutput, error) {
	if _, err := config.ParseTerminalVideo(string(preference)); err != nil {
		return "", config.VideoWindow, err
	}
	mpv, err := exec.LookPath("mpv")
	if err != nil {
		return "", config.VideoWindow, fmt.Errorf("mpv command not found; please install mpv: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, mpv, "--no-config", "--vo=help")
	cmd.WaitDelay = time.Second
	help, err := cmd.Output()
	if err != nil {
		return "", config.VideoWindow, fmt.Errorf("could not list mpv video outputs: %w", err)
	}
	output, err := selectTerminalOutput(preference, string(help), getenv, termgraphics.Current())
	return mpv, output, err
}

func selectTerminalOutput(preference config.VideoOutput, help string, getenv func(string) string, caps termgraphics.Capabilities) (config.VideoOutput, error) {
	available := make(map[config.VideoOutput]bool)
	for line := range strings.SplitSeq(help, "\n") {
		if fields := strings.Fields(line); len(fields) > 0 {
			available[config.VideoOutput(fields[0])] = true
		}
	}
	if preference != config.VideoAuto {
		if !available[preference] {
			return config.VideoWindow, fmt.Errorf("mpv does not provide the %s video output; check 'mpv --vo=help' or select another Terminal Video renderer in Settings", preference)
		}
		return preference, nil
	}

	term := strings.ToLower(getenv("TERM"))
	program := strings.ToLower(getenv("TERM_PROGRAM"))
	var candidates []config.VideoOutput
	if caps.Kitty {
		candidates = append(candidates, config.VideoKitty)
	}
	if caps.Sixel {
		candidates = append(candidates, config.VideoSixel)
	}
	// Multiplexers can hide capabilities or require passthrough configuration.
	// Do not emit graphics automatically based on the outer terminal's env vars.
	multiplexer := getenv("TMUX") != "" || getenv("STY") != "" || strings.HasPrefix(term, "screen") || strings.HasPrefix(term, "tmux")
	if !multiplexer && !caps.Responded {
		switch {
		case strings.Contains(term, "kitty"), getenv("KITTY_WINDOW_ID") != "", program == "kitty", program == "ghostty", term == "xterm-ghostty":
			candidates = append(candidates, config.VideoKitty)
		case getenv("KONSOLE_VERSION") != "":
			candidates = append(candidates, config.VideoKitty, config.VideoSixel)
		case strings.Contains(term, "sixel"), strings.HasPrefix(term, "foot"), strings.HasPrefix(term, "mlterm"), program == "wezterm", program == "contour", program == "mintty":
			candidates = append(candidates, config.VideoSixel)
		}
	}
	candidates = append(candidates, config.VideoTCT)
	for _, output := range candidates {
		if available[output] {
			return output, nil
		}
	}
	return config.VideoWindow, fmt.Errorf("no compatible terminal video output found; check 'mpv --vo=help' and select kitty, sixel, or tct in Settings")
}

// sixelSizeArgs bounds the pixel work without stretching the physical cells.
// Both the pixel size and matching cell geometry must describe the same area.
func sixelSizeArgs(limit, columns, rows, cellW, cellH int) []string {
	if cellW <= 0 || cellH <= 0 {
		cellW, cellH = 8, 16
	}
	columns, rows = max(1, columns), max(2, rows)
	width := columns * cellW
	if limit > 0 {
		width = min(width, limit)
	}
	cols := max(1, width/cellW)
	return []string{
		fmt.Sprintf("--vo-sixel-width=%d", width),
		fmt.Sprintf("--vo-sixel-height=%d", rows*cellH),
		fmt.Sprintf("--vo-sixel-cols=%d", cols),
		fmt.Sprintf("--vo-sixel-rows=%d", rows),
		fmt.Sprintf("--vo-sixel-left=%d", (columns-cols)/2+1),
	}
}

// Terminal VOs copy their private options at initialization. Updating option
// properties alone succeeds but leaves the active output using its old size.
// Recreate the VO after applying the complete geometry, preserving pause/time.
func resizeVideoOutput(output config.VideoOutput, args []string, graph string, call func(...any) error) error {
	for _, arg := range args {
		option, value, _ := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		if err := call("set_property", option, value); err != nil {
			return fmt.Errorf("resize terminal video: %w", err)
		}
	}
	// Assigning the same VO is optimized away by mpv; use null as a temporary
	// output so no window can open and the next assignment reloads the options.
	for _, vo := range []string{"null", string(output)} {
		if err := call("set_property", "vo", vo); err != nil {
			return fmt.Errorf("reconfigure terminal video: %w", err)
		}
	}
	// mpv's VO replacement reconnects native video tracks, but not a video
	// stream produced by lavfi-complex. Reapply that graph to reconnect music
	// visualizers too. Whitespace forces an update before restoring its value.
	if graph != "" {
		for _, value := range []string{graph + " ", graph} {
			if err := call("set_property", "lavfi-complex", value); err != nil {
				return fmt.Errorf("reconnect terminal visualizer: %w", err)
			}
		}
	}
	return nil
}

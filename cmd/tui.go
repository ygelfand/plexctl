package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/ygelfand/plexctl/internal/commands"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/termgraphics"
	"github.com/ygelfand/plexctl/internal/tui"
	"github.com/ygelfand/plexctl/internal/tui/artwork"
	"github.com/ygelfand/plexctl/internal/tui/player"
)

var tuiCmd = &cobra.Command{
	Use:     "tui",
	Short:   "Launch the interactive TUI",
	GroupID: "tui",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTUI(cmd)
	},
}

func runTUI(cmd *cobra.Command) (runErr error) {
	if err := checkMPV(cmd, false, false, true, true); err != nil {
		return err
	}
	config.IsTUI = true
	cfg := config.Get()
	// Always log TUI sessions to a file for easier debugging
	cfg.LogFile = filepath.Join(cfg.CacheDir, "tui.log")
	cfg.SetupLogging()
	slog.Info("TUI Starting", "log_file", cfg.LogFile, "verbosity", cfg.Verbosity)

	if err := commands.EnsureActiveServer(context.Background()); err != nil {
		slog.Error("TUI: Failed to ensure active server", "error", err)
		return err
	}

	resultChan := make(chan plex.LoaderResult, 1)
	caps := termgraphics.Probe(os.Stdin, os.Stdout)
	slog.Info("Terminal graphics", "terminal", caps.Name, "kitty", caps.Kitty, "sixel", caps.Sixel,
		"native_posters", caps.NativePosters(), "shared_memory", caps.KittySharedMemory, "tmux", termgraphics.InTmux())
	restore, err := termgraphics.EnablePassthrough()
	if err != nil {
		slog.Debug("Graphics passthrough unavailable", "error", err)
	}
	defer restore()
	output, err := artwork.OpenRenderer(os.Stdout, cfg.PosterRenderer, caps)
	if err != nil {
		return fmt.Errorf("initialize poster graphics: %w", err)
	}
	defer func() {
		if err := output.Close(); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("close poster graphics: %w", err))
		}
	}()

	slog.Debug("TUI: Showing standalone splash")
	tui.ShowStandaloneSplash(1*time.Second, resultChan)

	slog.Debug("TUI: Waiting for loader result")
	result := <-resultChan
	slog.Debug("TUI: Received loader result", "libraries", len(result.Libraries))

	slog.Debug("TUI: Initializing controller and program")
	p := tea.NewProgram(tui.NewController(result), tea.WithAltScreen(), tea.WithOutput(output), tea.WithMouseCellMotion())
	defer func() {
		pm := player.GetPlayerManager()
		if pm.Status().AudioOnly || pm.Status().Overlay || cfg.CloseVideoOnQuit {
			pm.StopPlayback()()
		}
		pm.Close()
	}()

	if _, err := p.Run(); err != nil {
		slog.Error("TUI: Program run failed", "error", err)
		return fmt.Errorf("failed to run TUI: %w", err)
	}
	slog.Info("TUI Finished normally")
	return nil
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}

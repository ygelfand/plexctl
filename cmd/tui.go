package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/ygelfand/plexctl/internal/commands"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/tui"
)

var tuiCmd = &cobra.Command{
	Use:     "tui",
	Short:   "Launch the interactive TUI",
	GroupID: "tui",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runTUI()
	},
}

func runTUI() error {
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

	slog.Debug("TUI: Showing standalone splash")
	tui.ShowStandaloneSplash(1*time.Second, resultChan)

	slog.Debug("TUI: Waiting for loader result")
	result := <-resultChan
	slog.Debug("TUI: Received loader result", "libraries", len(result.Libraries))

	slog.Debug("TUI: Initializing controller and program")
	p := tea.NewProgram(tui.NewController(result), tea.WithAltScreen())

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

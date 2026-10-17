package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/ygelfand/plexctl/internal/commands"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/tui/player"
)

var (
	tctMode  bool
	noResume bool
	trailer  bool
)

var playCmd = &cobra.Command{
	Use:     "play [media_id]",
	Short:   "Play a media item",
	Args:    cobra.ExactArgs(1),
	GroupID: "media",
	RunE: commands.RunWithServer(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		if err := checkMPV(cmd, true, false, true, true); err != nil {
			return err
		}
		output := config.VideoWindow
		if tctMode {
			output = config.VideoTCT
		} else if cmd.Flags().Changed("terminal-video") {
			output = config.Get().TerminalVideo
		}
		if output != config.VideoWindow {
			// Progress-report diagnostics must not overwrite terminal graphics.
			cfg := config.Get()
			cfg.LogFile = filepath.Join(cfg.CacheDir, "player.log")
			cfg.SetupLogging()
		} else {
			var cancel context.CancelFunc
			ctx, cancel = signal.NotifyContext(ctx, os.Interrupt)
			defer cancel()
		}
		mediaID := args[0]
		slog.Debug("CLI Play: Fetching metadata", "mediaID", mediaID)

		meta, err := plex.GetMetadata(ctx, mediaID, true)
		if err != nil {
			return fmt.Errorf("failed to get metadata: %w", err)
		}

		var playFunc tea.Cmd

		if trailer {
			slog.Debug("CLI Play: Resolving trailer", "mediaID", mediaID)
			playFunc = player.FetchAndPlayTrailer(ctx, meta, output)
		} else {
			offset := int64(0)
			if !noResume && meta.Type != "artist" && meta.Type != "album" && meta.ViewOffset != nil {
				offset = int64(*meta.ViewOffset)
			}

			if offset > 0 {
				slog.Info("Resuming", "title", meta.Title, "type", meta.Type, "offset", offset)
			} else {
				slog.Info("Playing", "title", meta.Title, "type", meta.Type)
			}

			playFunc = player.PlayMedia(ctx, meta, false, output, offset)
		}

		if playFunc == nil {
			return fmt.Errorf("failed to initiate playback")
		}

		// Execute the playback function
		pm := player.GetPlayerManager()
		defer pm.Close()
		defer func() { pm.StopPlayback()() }()
		msg := playFunc()
		if err, ok := msg.(error); ok {
			return err
		}
		if fallback, ok := msg.(player.VideoFallbackMsg); ok {
			if _, err := fmt.Fprintln(cmd.ErrOrStderr(), fallback.String()); err != nil {
				return err
			}
		}
		if output != config.VideoWindow {
			// Terminal playback owns the foreground until EOF or the user's q.
			return nil
		}
		title := meta.Title
		if trailer {
			title = title + "(Trailer)"
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Playing %s. Press Ctrl+C to stop.\n", title); err != nil {
			return fmt.Errorf("write playback status: %w", err)
		}

		// Wait loop for CLI to keep progress reporting alive
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for pm.Status().Active() {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				// Just keep waiting
			}
		}

		return nil
	}),
}

func init() {
	rootCmd.AddCommand(playCmd)
	playCmd.Flags().BoolVar(&tctMode, "tct", false, "Use the TCT text renderer (legacy terminal video)")
	playCmd.Flags().BoolVar(&noResume, "no-resume", false, "Start playback from the beginning")
	playCmd.Flags().BoolVar(&trailer, "trailer", false, "Play the primary trailer instead of the full media")
}

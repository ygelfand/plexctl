package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/ygelfand/plexctl/internal/commands"
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
		mediaID := args[0]
		slog.Debug("CLI Play: Fetching metadata", "mediaID", mediaID)

		meta, err := plex.GetMetadata(ctx, mediaID, true)
		if err != nil {
			return fmt.Errorf("failed to get metadata: %w", err)
		}

		var playFunc tea.Cmd

		if trailer {
			slog.Debug("CLI Play: Resolving trailer", "mediaID", mediaID)
			playFunc = player.FetchAndPlayTrailer(meta, tctMode)
		} else {
			offset := int64(0)
			if !noResume && meta.ViewOffset != nil {
				offset = int64(*meta.ViewOffset)
			}

			if offset > 0 {
				slog.Info("Resuming", "title", meta.Title, "type", meta.Type, "offset", offset)
			} else {
				slog.Info("Playing", "title", meta.Title, "type", meta.Type)
			}

			playFunc = player.PlayMedia(meta, false, tctMode, offset)
		}

		if playFunc == nil {
			return fmt.Errorf("failed to initiate playback")
		}

		// Execute the playback function
		msg := playFunc()
		if err, ok := msg.(error); ok {
			return err
		}
		title := meta.Title
		if trailer {
			title = title + "(Trailer)"
		}
		pm := player.GetPlayerManager()
		if tctMode {
			fmt.Printf("Playing %s in TCT mode. Press 'q' in mpv or Ctrl+C to stop.\n", title)
		} else {
			fmt.Printf("Playing %s . Press Ctrl+C to stop.\n", title)
		}

		// Wait loop for CLI to keep progress reporting alive
		for pm.VerifyConnection() {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(500 * time.Millisecond):
				// Just keep waiting
			}
		}

		return nil
	}),
}

func init() {
	rootCmd.AddCommand(playCmd)
	playCmd.Flags().BoolVar(&tctMode, "tct", false, "Use terminal video")
	playCmd.Flags().BoolVar(&noResume, "no-resume", false, "Start playback from the beginning")
	playCmd.Flags().BoolVar(&trailer, "trailer", false, "Play the primary trailer instead of the full media")
}

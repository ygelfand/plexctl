package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/spf13/cobra"
	"github.com/ygelfand/plexctl/internal/commands"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/tui/player"
)

var (
	tctMode  bool
	noResume bool
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

		offset := int64(0)
		if !noResume && meta.ViewOffset != nil {
			offset = int64(*meta.ViewOffset)
		}

		if offset > 0 {
			slog.Info("Resuming", "title", meta.Title, "type", meta.Type, "offset", offset)
		} else {
			slog.Info("Playing", "title", meta.Title, "type", meta.Type)
		}

		// player.PlayMedia returns a tea.Cmd, which is func() tea.Msg
		playFunc := player.PlayMedia(meta, false, tctMode, offset)
		if playFunc == nil {
			return fmt.Errorf("failed to initiate playback")
		}

		playFunc()

		pm := player.GetPlayerManager()
		if tctMode {
			fmt.Printf("Playing %s in TCT mode. Press 'q' in mpv or Ctrl+C to stop.\n", meta.Title)
		} else {
			fmt.Printf("Playing %s.Press Ctrl+C to stop.\n", meta.Title)
		}

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
}

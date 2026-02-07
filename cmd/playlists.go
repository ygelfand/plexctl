package cmd

import (
	"context"
	"fmt"
	"strconv"

	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/spf13/cobra"
	"github.com/ygelfand/plexctl/internal/commands"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/presenters"
)

var playlistCmd = &cobra.Command{
	Use:     "playlist",
	Short:   "Manage playlists",
	GroupID: "media",
}

var playlistListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all playlists",
	RunE: commands.RunWithServer(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		res, err := client.SDK.Playlist.ListPlaylists(ctx, operations.ListPlaylistsRequest{})
		if err != nil {
			return err
		}

		if res.MediaContainerWithPlaylistMetadata == nil || res.MediaContainerWithPlaylistMetadata.MediaContainer == nil || len(res.MediaContainerWithPlaylistMetadata.MediaContainer.Metadata) == 0 {
			fmt.Println("No playlists found.")
			return nil
		}

		return commands.Print(&presenters.LibraryItemsPresenter{
			SectionID: "Playlists",
			Items:     presenters.MapPlaylistMetadata(res.MediaContainerWithPlaylistMetadata.MediaContainer.Metadata),
			RawData:   res.MediaContainerWithPlaylistMetadata.MediaContainer.Metadata,
		}, opts)
	}),
}

var playlistShowCmd = &cobra.Command{
	Use:   "show [playlist_id]",
	Short: "Show items in a playlist",
	Args:  cobra.ExactArgs(1),
	RunE: commands.RunWithServer(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		playlistID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid playlist ID: %w", err)
		}

		res, err := client.SDK.Playlist.GetPlaylistItems(ctx, operations.GetPlaylistItemsRequest{
			PlaylistID: playlistID,
		})
		if err != nil {
			return err
		}

		if res.MediaContainerWithMetadata == nil || res.MediaContainerWithMetadata.MediaContainer == nil || len(res.MediaContainerWithMetadata.MediaContainer.Metadata) == 0 {
			fmt.Println("No items found in this playlist.")
			return nil
		}

		return commands.Print(&presenters.LibraryItemsPresenter{
			SectionID: fmt.Sprintf("Playlist %d", playlistID),
			Items:     presenters.MapMetadata(res.MediaContainerWithMetadata.MediaContainer.Metadata),
			RawData:   res.MediaContainerWithMetadata.MediaContainer.Metadata,
		}, opts)
	}),
}

func init() {
	rootCmd.AddCommand(playlistCmd)
	playlistCmd.AddCommand(playlistListCmd)
	playlistCmd.AddCommand(playlistShowCmd)
}

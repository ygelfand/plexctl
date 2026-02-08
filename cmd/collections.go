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

var collectionCmd = &cobra.Command{
	Use:     "collection",
	Short:   "Manage collections",
	GroupID: "media",
}

var collectionListCmd = &cobra.Command{
	Use:   "list [library_id]",
	Short: "List collections in a library",
	Args:  cobra.ExactArgs(1),
	RunE: commands.RunWithServer(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		libraryID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid library ID: %w", err)
		}

		res, err := client.SDK.Library.GetCollections(ctx, operations.GetCollectionsRequest{
			SectionID: libraryID,
		})
		if err != nil {
			return err
		}

		if res.MediaContainerWithMetadata == nil || res.MediaContainerWithMetadata.MediaContainer == nil || len(res.MediaContainerWithMetadata.MediaContainer.Metadata) == 0 {
			fmt.Println("No collections found in this library.")
			return nil
		}

		return commands.Print(&presenters.CollectionsPresenter{
			SectionID:   args[0],
			Collections: res.MediaContainerWithMetadata.MediaContainer.Metadata,
			RawData:     res.MediaContainerWithMetadata.MediaContainer.Metadata,
		}, opts)
	}),
}

var collectionShowCmd = &cobra.Command{
	Use:   "show [collection_id]",
	Short: "Show items in a collection",
	Args:  cobra.ExactArgs(1),
	RunE: commands.RunWithServer(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		collectionID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid collection ID: %w", err)
		}

		res, err := client.SDK.Content.GetCollectionItems(ctx, operations.GetCollectionItemsRequest{
			CollectionID: collectionID,
		})
		if err != nil {
			return err
		}

		if res.MediaContainerWithMetadata == nil || res.MediaContainerWithMetadata.MediaContainer == nil || len(res.MediaContainerWithMetadata.MediaContainer.Metadata) == 0 {
			fmt.Println("No items found in this collection.")
			return nil
		}

		return commands.Print(&presenters.LibraryItemsPresenter{
			SectionID: fmt.Sprintf("Collection %d", collectionID),
			Items:     presenters.MapMetadata(res.MediaContainerWithMetadata.MediaContainer.Metadata),
			RawData:   res.MediaContainerWithMetadata.MediaContainer.Metadata,
		}, opts)
	}),
}

func init() {
	rootCmd.AddCommand(collectionCmd)
	collectionCmd.AddCommand(collectionListCmd)
	collectionCmd.AddCommand(collectionShowCmd)
}

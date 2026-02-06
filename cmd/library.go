package cmd

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/spf13/cobra"
	"github.com/ygelfand/plexctl/internal/commands"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/presenters"
)

var libraryCmd = &cobra.Command{
	Use:     "library",
	Short:   "Manage libraries",
	GroupID: "media",
}

var libraryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all libraries",
	RunE: commands.RunWithServer(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		slog.Debug("SDK: Fetching sections")
		res, err := client.SDK.Library.GetSections(ctx)
		if err != nil {
			slog.Error("SDK: Failed to get sections", "error", err)
			return fmt.Errorf("failed to get sections: %w", err)
		}

		if res.Object == nil || res.Object.MediaContainer == nil || len(res.Object.MediaContainer.Directory) == 0 {
			slog.Debug("SDK: No sections found")
			fmt.Println("No libraries found.")
			return nil
		}

		slog.Debug("SDK: Found sections", "count", len(res.Object.MediaContainer.Directory))
		return commands.Print(&presenters.LibraryListPresenter{
			Directories: res.Object.MediaContainer.Directory,
		}, opts)
	}),
}

var libraryShowCmd = &cobra.Command{
	Use:   "show [section_id]",
	Short: "Show items in a library section",
	Args:  cobra.ExactArgs(1),
	RunE: commands.RunWithServer(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		sectionID := args[0]
		slog.Debug("SDK: Fetching library content", "section_id", sectionID)

		res, err := client.SDK.Content.ListContent(ctx, operations.ListContentRequest{
			SectionID: sectionID,
		})
		if err != nil {
			slog.Error("SDK: Failed to get library items", "section_id", sectionID, "error", err)
			return fmt.Errorf("failed to get library items: %w", err)
		}

		if res.MediaContainerWithMetadata == nil || res.MediaContainerWithMetadata.MediaContainer == nil || len(res.MediaContainerWithMetadata.MediaContainer.Metadata) == 0 {
			slog.Debug("SDK: No items found", "section_id", sectionID)
			fmt.Println("No items found in this library.")
			return nil
		}

		slog.Debug("SDK: Found items", "section_id", sectionID, "count", len(res.MediaContainerWithMetadata.MediaContainer.Metadata))
		return commands.Print(&presenters.LibraryItemsPresenter{
			SectionID: sectionID,
			Metadata:  res.MediaContainerWithMetadata.MediaContainer.Metadata,
		}, opts)
	}),
}

func init() {
	rootCmd.AddCommand(libraryCmd)
	libraryCmd.AddCommand(libraryListCmd)
	libraryCmd.AddCommand(libraryShowCmd)
}

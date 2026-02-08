package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/spf13/cobra"
	"github.com/ygelfand/plexctl/internal/commands"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/presenters"
	"github.com/ygelfand/plexctl/internal/ui"
)

var (
	libraryCount int
	libraryPage  int
	libraryAll   bool
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
	Use:   "show [library_id]",
	Short: "Show items in a library",
	Args:  cobra.ExactArgs(1),
	RunE: commands.RunWithServer(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		libraryID := args[0]
		slog.Debug("SDK: Fetching library content", "library_id", libraryID)

		allMetadata, err := plex.WalkContent(ctx, libraryAll, libraryPage, libraryCount, plex.LibraryWalker(client, libraryID))
		if err != nil {
			return err
		}

		if len(allMetadata) == 0 {
			fmt.Println("No items found in this library.")
			return nil
		}

		slog.Debug("SDK: Found items", "library_id", libraryID, "count", len(allMetadata))
		return commands.Print(&presenters.LibraryItemsPresenter{
			SectionID: libraryID,
			Items:     presenters.MapMetadata(allMetadata),
			RawData:   allMetadata,
		}, opts)
	}),
}

var libraryRefreshCmd = &cobra.Command{
	Use:   "refresh [library_id]",
	Short: "Trigger a metadata refresh for a library",
	Args:  cobra.ExactArgs(1),
	RunE: commands.RunWithServer(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		libraryID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid library ID: %w", err)
		}

		res, err := client.SDK.Library.RefreshSection(ctx, operations.RefreshSectionRequest{
			SectionID: libraryID,
		})
		if err != nil {
			return err
		}

		if res.StatusCode == 200 {
			ui.RenderSuccess(fmt.Sprintf("Refresh triggered for library %d", libraryID))
		}
		return nil
	}),
}

func init() {
	rootCmd.AddCommand(libraryCmd)
	libraryCmd.AddCommand(libraryListCmd)
	libraryCmd.AddCommand(libraryShowCmd)
	libraryCmd.AddCommand(libraryRefreshCmd)

	libraryShowCmd.Flags().IntVar(&libraryCount, "count", 50, "Number of items to return per page")
	libraryShowCmd.Flags().IntVar(&libraryPage, "page", 1, "Page number to return")
	libraryShowCmd.Flags().BoolVar(&libraryAll, "all", false, "Return all items (overrides count/page)")
}

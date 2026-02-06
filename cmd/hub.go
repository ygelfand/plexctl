package cmd

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/ygelfand/plexctl/internal/commands"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/presenters"
)

var hubCmd = &cobra.Command{
	Use:     "hub",
	Short:   "Manage hubs",
	GroupID: "media",
}

var hubListCmd = &cobra.Command{
	Use:   "list",
	Short: "List promoted hubs",
	RunE: commands.RunWithServer(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		slog.Debug("SDK: Fetching home hubs")
		hubs, err := plex.GetHomeHubs(ctx)
		if err != nil {
			slog.Error("SDK: Failed to get home hubs", "error", err)
			return err
		}

		if len(hubs) == 0 {
			slog.Debug("SDK: No hubs found")
			fmt.Println("No hubs found.")
			return nil
		}

		slog.Debug("SDK: Found hubs", "count", len(hubs))
		headers := []string{"TITLE", "TYPE", "ID"}
		var rows [][]string

		for _, hub := range hubs {
			title := ""
			if hub.Title != nil {
				title = *hub.Title
			}

			typ := ""
			if hub.Type != nil {
				typ = *hub.Type
			}

			id := ""
			if hub.HubIdentifier != nil {
				id = *hub.HubIdentifier
			}

			rows = append(rows, []string{title, typ, id})
		}

		return commands.Print(presenters.SimplePresenter{
			T:       "Promoted Hubs",
			H:       headers,
			R:       rows,
			RawData: hubs,
		}, opts)
	}),
}

func init() {
	rootCmd.AddCommand(hubCmd)
	hubCmd.AddCommand(hubListCmd)
}

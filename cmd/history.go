package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/spf13/cobra"
	"github.com/ygelfand/plexctl/internal/commands"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/presenters"
)

var since string

var historyCmd = &cobra.Command{
	Use:     "history",
	Short:   "Show playback history",
	GroupID: "media",
	RunE: commands.RunWithServer(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		slog.Debug("SDK: Fetching history", "since", since)

		var viewedAt *int64
		if since != "" {
			d, err := time.ParseDuration(since)
			if err != nil {
				if strings.HasSuffix(since, "d") {
					val := strings.TrimSuffix(since, "d")
					var days int
					fmt.Sscanf(val, "%d", &days)
					d = time.Duration(days) * 24 * time.Hour
				} else if strings.HasSuffix(since, "w") {
					val := strings.TrimSuffix(since, "w")
					var weeks int
					fmt.Sscanf(val, "%d", &weeks)
					d = time.Duration(weeks) * 7 * 24 * time.Hour
				} else {
					return fmt.Errorf("invalid duration format (use 1h, 1d, 1w): %w", err)
				}
			}
			ts := time.Now().Add(-d).Unix()
			viewedAt = &ts
		}

		userMap := make(map[int64]string)
		uRes, err := client.SDK.Users.GetUsers(ctx, operations.GetUsersRequest{})
		if err == nil && uRes.Object != nil && uRes.Object.MediaContainer != nil {
			for _, u := range uRes.Object.MediaContainer.User {
				userMap[u.ID] = u.Title
			}
		}

		libMap := make(map[string]string)
		lRes, err := client.SDK.Library.GetSections(ctx)
		if err == nil && lRes.Object != nil && lRes.Object.MediaContainer != nil {
			for _, l := range lRes.Object.MediaContainer.Directory {
				if l.Key != nil && l.Title != nil {
					libMap[*l.Key] = *l.Title
				}
			}
		}

		deviceMap := make(map[string]string)
		dRes, err := client.SDK.Plex.GetServerResources(ctx, operations.GetServerResourcesRequest{})
		if err == nil {
			for _, d := range dRes.PlexDevices {
				deviceMap[d.ClientIdentifier] = d.Name
			}
		}

		res, err := client.SDK.Status.ListPlaybackHistory(ctx, operations.ListPlaybackHistoryRequest{
			Sort:        []string{"viewedAt:desc"},
			ViewedAtGte: viewedAt,
		})
		if err != nil {
			return err
		}

		if res.Object == nil || res.Object.MediaContainer == nil || len(res.Object.MediaContainer.Metadata) == 0 {
			fmt.Println("No history found.")
			return nil
		}

		items := presenters.MapHistoryMetadata(res.Object.MediaContainer.Metadata, userMap, libMap, deviceMap)

		return commands.Print(&presenters.HistoryPresenter{
			Items:   items,
			RawData: res.Object.MediaContainer.Metadata,
		}, opts)
	}),
}

func init() {
	rootCmd.AddCommand(historyCmd)
	historyCmd.Flags().StringVar(&since, "since", "1w", "Time period to show (e.g. 1h, 1d, 1w)")
}

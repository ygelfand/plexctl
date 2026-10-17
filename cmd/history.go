package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strconv"
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
			d, err := parseHistoryDuration(since)
			if err != nil {
				return fmt.Errorf("invalid duration format (use 1h, 1d, 1w): %w", err)
			}
			ts := time.Now().Add(-d).Unix()
			viewedAt = &ts
		}

		userMap := make(map[int64]string)
		account, accountErr := plex.NewAccountClient()
		if accountErr == nil {
			uRes, err := account.SDK.Users.GetUsers(ctx, operations.GetUsersRequest{})
			if err == nil && uRes.Object != nil && uRes.Object.MediaContainer != nil {
				for _, u := range uRes.Object.MediaContainer.User {
					userMap[u.ID] = u.Title
				}
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
		if accountErr == nil {
			dRes, err := account.SDK.Plex.GetServerResources(ctx, operations.GetServerResourcesRequest{})
			if err == nil {
				for _, d := range dRes.PlexDevices {
					deviceMap[d.ClientIdentifier] = d.Name
				}
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

func parseHistoryDuration(value string) (time.Duration, error) {
	var unit time.Duration
	switch {
	case strings.HasSuffix(value, "d"):
		unit = 24 * time.Hour
	case strings.HasSuffix(value, "w"):
		unit = 7 * 24 * time.Hour
	default:
		return time.ParseDuration(value)
	}
	count, err := strconv.ParseInt(value[:len(value)-1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse duration %q: %w", value, err)
	}
	if count > math.MaxInt64/int64(unit) || count < math.MinInt64/int64(unit) {
		return 0, fmt.Errorf("duration %q is out of range", value)
	}
	return time.Duration(count) * unit, nil
}

func init() {
	rootCmd.AddCommand(historyCmd)
	historyCmd.Flags().StringVar(&since, "since", "1w", "Time period to show (e.g. 1h, 1d, 1w)")
}

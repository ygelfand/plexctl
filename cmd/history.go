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
		// Resolve since filter
		var viewedAt *int64
		if since != "" {
			d, err := time.ParseDuration(since)
			if err != nil {
				// Handle days/weeks manually if ParseDuration doesn't (it only does h, m, s)
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
					slog.Error("History: invalid duration format", "since", since, "error", err)
					return fmt.Errorf("invalid duration format (use 1h, 1d, 1w): %w", err)
				}
			}
			ts := time.Now().Add(-d).Unix()
			viewedAt = &ts
		}

		// Resolve Users
		slog.Debug("SDK: Resolving user map")
		userMap := make(map[int64]string)
		uRes, err := client.SDK.Users.GetUsers(ctx, operations.GetUsersRequest{})
		if err == nil && uRes.Object != nil && uRes.Object.MediaContainer != nil {
			for _, u := range uRes.Object.MediaContainer.User {
				userMap[u.ID] = u.Title
			}
		}

		// Resolve Libraries
		slog.Debug("SDK: Resolving library map")
		libMap := make(map[string]string)
		lRes, err := client.SDK.Library.GetSections(ctx)
		if err == nil && lRes.Object != nil && lRes.Object.MediaContainer != nil {
			for _, l := range lRes.Object.MediaContainer.Directory {
				if l.Key != nil && l.Title != nil {
					libMap[*l.Key] = *l.Title
				}
			}
		}

		// Resolve Devices
		slog.Debug("SDK: Resolving device map")
		deviceMap := make(map[string]string)
		dRes, err := client.SDK.Devices.ListDevices(ctx)
		if err == nil && dRes.MediaContainerWithDevice != nil && dRes.MediaContainerWithDevice.MediaContainer != nil {
			for _, d := range dRes.MediaContainerWithDevice.MediaContainer.Device {
				if d.UUID != nil {
					name := "Unknown"
					if d.Model != nil {
						name = *d.Model
					} else if d.Make != nil {
						name = *d.Make
					}
					deviceMap[*d.UUID] = name
				}
			}
		}

		slog.Debug("SDK: Executing history list request")
		res, err := client.SDK.Status.ListPlaybackHistory(ctx, operations.ListPlaybackHistoryRequest{
			Sort:        []string{"viewedAt:desc"},
			ViewedAtGte: viewedAt,
		})
		if err != nil {
			slog.Error("SDK: Failed to get playback history", "error", err)
			return err
		}

		if res.Object == nil || res.Object.MediaContainer == nil {
			slog.Debug("SDK: No history data received")
			fmt.Println("No history found.")
			return nil
		}

		slog.Debug("SDK: Received history items", "count", len(res.Object.MediaContainer.Metadata))

		headers := []string{"DATE", "USER", "TITLE", "TYPE", "DEVICE", "LIBRARY"}
		var rows [][]string
		for _, meta := range res.Object.MediaContainer.Metadata {
			viewedAt := ""
			if meta.ViewedAt != nil {
				viewedAt = time.Unix(*meta.ViewedAt, 0).Format("2006-01-02 15:04")
			}

			user := "Unknown"
			if meta.AccountID != nil {
				if u, ok := userMap[*meta.AccountID]; ok {
					user = u
				} else {
					user = fmt.Sprintf("%d", *meta.AccountID)
				}
			}

			title := ""
			if meta.Title != nil {
				title = *meta.Title
			}

			mType := ""
			if meta.Type != nil {
				mType = *meta.Type
			}

			device := "Unknown"
			if meta.DeviceID != nil {
				dKey := fmt.Sprintf("%d", *meta.DeviceID)
				if d, ok := deviceMap[dKey]; ok {
					device = d
				} else {
					device = dKey
				}
			}

			library := "Unknown"
			if meta.LibrarySectionID != nil {
				if l, ok := libMap[*meta.LibrarySectionID]; ok {
					library = l
				} else {
					library = *meta.LibrarySectionID
				}
			}

			rows = append(rows, []string{viewedAt, user, title, mType, device, library})
		}

		return commands.Print(presenters.SimplePresenter{
			T:       "Playback History",
			H:       headers,
			R:       rows,
			RawData: res.Object.MediaContainer.Metadata,
		}, opts)
	}),
}

func init() {
	rootCmd.AddCommand(historyCmd)
	historyCmd.Flags().StringVar(&since, "since", "1w", "Time period to show (e.g. 1h, 1d, 1w)")
}

package cmd

import (
	"context"
	"fmt"

	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/spf13/cobra"
	"github.com/ygelfand/plexctl/internal/commands"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/presenters"
)

var usersCmd = &cobra.Command{
	Use:     "users",
	Short:   "Manage Plex users",
	GroupID: "media",
}

var usersListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all users",
	RunE: commands.RunWithServer(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		res, err := client.SDK.Users.GetUsers(ctx, operations.GetUsersRequest{})
		if err != nil {
			return err
		}

		if res.Object == nil || res.Object.MediaContainer == nil {
			return nil
		}

		headers := []string{"ID", "USERNAME", "EMAIL", "TITLE"}
		var rows [][]string
		for _, u := range res.Object.MediaContainer.User {
			rows = append(rows, []string{
				fmt.Sprintf("%d", u.ID),
				u.Username,
				u.Email,
				u.Title,
			})
		}

		return commands.Print(presenters.SimplePresenter{
			T:       "Plex Users",
			H:       headers,
			R:       rows,
			RawData: res.Object.MediaContainer.User,
		}, opts)
	}),
}

func init() {
	rootCmd.AddCommand(usersCmd)
	usersCmd.AddCommand(usersListCmd)
}

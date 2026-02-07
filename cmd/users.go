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

		if res.Object == nil || res.Object.MediaContainer == nil || len(res.Object.MediaContainer.User) == 0 {
			fmt.Println("No users found.")
			return nil
		}

		return commands.Print(&presenters.UsersPresenter{
			Users: res.Object.MediaContainer.User,
		}, opts)
	}),
}

func init() {
	rootCmd.AddCommand(usersCmd)
	usersCmd.AddCommand(usersListCmd)
}

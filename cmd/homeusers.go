package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/ygelfand/plexctl/internal/commands"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/presenters"
)

var homeUsersCmd = &cobra.Command{
	Use:     "homeusers",
	Short:   "Manage Plex Home users",
	GroupID: "media",
}

var homeUsersListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all users in the Plex Home",
	RunE: commands.RunWithServer(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		res, err := client.SDK.HomeUsers.GetHomeUsers(ctx)
		if err != nil {
			return err
		}

		if res.Object == nil || len(res.Object.Users) == 0 {
			fmt.Println("No home users found.")
			return nil
		}

		return commands.Print(&presenters.HomeUsersPresenter{
			Users: res.Object.Users,
		}, opts)
	}),
}

func init() {
	rootCmd.AddCommand(homeUsersCmd)
	homeUsersCmd.AddCommand(homeUsersListCmd)
}

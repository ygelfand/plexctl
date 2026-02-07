package cmd

import (
	"context"
	"fmt"

	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/spf13/cobra"
	"github.com/ygelfand/plexctl/internal/commands"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/presenters"
	"github.com/ygelfand/plexctl/internal/ui"
)

var deviceCmd = &cobra.Command{
	Use:     "device",
	Short:   "Manage account devices",
	GroupID: "media",
}

var deviceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all devices associated with this account",
	RunE: commands.RunWithServer(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		res, err := client.SDK.Plex.GetServerResources(ctx, operations.GetServerResourcesRequest{})
		if err != nil {
			return err
		}

		if len(res.PlexDevices) == 0 {
			fmt.Println("No devices found.")
			return nil
		}

		return commands.Print(&presenters.DevicesPresenter{
			Devices: res.PlexDevices,
		}, opts)
	}),
}

var deviceShowCmd = &cobra.Command{
	Use:   "show [device_id]",
	Short: "Show detailed information for a device",
	Args:  cobra.ExactArgs(1),
	RunE: commands.RunWithServer(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		deviceID := args[0]
		res, err := client.SDK.Plex.GetServerResources(ctx, operations.GetServerResourcesRequest{})
		if err != nil {
			return err
		}

		for _, d := range res.PlexDevices {
			if d.ClientIdentifier == deviceID {
				ui.RenderSummary(fmt.Sprintf("Device: %s", d.Name), []struct{ Label, Value string }{
					{"ID", d.ClientIdentifier},
					{"Product", d.Product},
					{"Version", d.ProductVersion},
					{"Platform", ui.PtrToString(d.Platform)},
					{"Provides", d.Provides},
					{"Last Seen", d.LastSeenAt.Format("2006-01-02 15:04")},
					{"Public IP", d.PublicAddress},
				})
				return nil
			}
		}

		return fmt.Errorf("device %s not found", deviceID)
	}),
}

func init() {
	rootCmd.AddCommand(deviceCmd)
	deviceCmd.AddCommand(deviceListCmd)
	deviceCmd.AddCommand(deviceShowCmd)
}

package cmd

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/spf13/cobra"
	"github.com/ygelfand/plexctl/internal/commands"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/presenters"
	"github.com/ygelfand/plexctl/internal/ui"
)

var serverCmd = &cobra.Command{
	Use:     "server",
	Short:   "Manage Plex Server",
	GroupID: "media",
}

var serverStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Get server status",
	RunE: commands.RunWithServer(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		slog.Debug("SDK: Fetching server identity")
		res, err := client.SDK.General.GetIdentity(ctx)
		if err != nil {
			slog.Error("SDK: Failed to get identity", "error", err)
			return fmt.Errorf("failed to get identity: %w", err)
		}

		if res.Object == nil || res.Object.MediaContainer == nil {
			slog.Error("SDK: No identity information received")
			return fmt.Errorf("no identity information received")
		}

		slog.Debug("SDK: Received identity", "machine_identifier", ui.PtrToString(res.Object.MediaContainer.MachineIdentifier))
		return commands.Print(&presenters.ServerIdentityPresenter{
			Container: res.Object.MediaContainer,
		}, opts)
	}),
}

var serverListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available Plex servers from plex.tv",
	Annotations: map[string]string{
		ui.AnnotationSkipServerCheck: "true",
	},
	RunE: commands.RunWithClient(func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *commands.PlexCtlOptions) error {
		slog.Debug("SDK: Fetching server resources from plex.tv")
		res, err := client.SDK.Plex.GetServerResources(ctx, operations.GetServerResourcesRequest{
			IncludeHTTPS: operations.IncludeHTTPSTrue.ToPointer(),
			IncludeIPv6:  operations.IncludeIPv6True.ToPointer(),
			IncludeRelay: operations.IncludeRelayTrue.ToPointer(),
		})
		if err != nil {
			slog.Error("SDK: Failed to list servers", "error", err)
			return fmt.Errorf("failed to list servers: %w", err)
		}

		slog.Debug("SDK: Received server resources", "count", len(res.PlexDevices))
		return commands.Print(&presenters.ServerListPresenter{
			Devices: res.PlexDevices,
		}, opts)
	}),
}

var serverDiscoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Interactive discovery and selection of a Plex server",
	Annotations: map[string]string{
		ui.AnnotationSkipServerCheck: "true",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		slog.Debug("Server: Starting interactive discovery")
		return commands.DiscoverAndSelectServer(cmd.Context())
	},
}

var serverUseCmd = &cobra.Command{
	Use:   "use [name_or_id]",
	Short: "Switch to a different configured server",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Get()
		slog.Debug("Config: Switching default server", "target", args[0])
		if err := cfg.SetDefaultServer(args[0]); err != nil {
			slog.Warn("Config: Failed to switch server", "target", args[0], "error", err)
			return err
		}
		if err := cfg.Save(); err != nil {
			slog.Error("Config: Failed to save after server switch", "error", err)
			return err
		}
		slog.Info("Config: Default server switched successfully", "default", cfg.DefaultServer)
		ui.RenderSuccess(fmt.Sprintf("Now using server: %s", cfg.DefaultServer))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
	serverCmd.AddCommand(serverStatusCmd)
	serverCmd.AddCommand(serverListCmd)
	serverCmd.AddCommand(serverDiscoverCmd)
	serverCmd.AddCommand(serverUseCmd)
}

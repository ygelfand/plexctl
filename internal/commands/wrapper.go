package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/presenters"
	"github.com/ygelfand/plexctl/internal/ui"
)

// RunnerFunc defines the signature for a command handler that receives a Plex client
type RunnerFunc func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *PlexCtlOptions) error

// RunWithClient wraps a cobra command RunE function to inject a configured Plex client
func RunWithClient(runner RunnerFunc) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		opts := &PlexCtlOptions{
			OutputFormat: viper.GetString("output"),
			Verbosity:    viper.GetInt("verbose"),
			Sort:         viper.GetString("sort"),
		}

		client, err := plex.NewClient()
		if err != nil {
			return err
		}
		return runner(cmd.Context(), client, cmd, args, opts)
	}
}

// RunWithServer wraps a cobra command RunE function to inject a configured Plex client
// and ensures that a default server has been selected.
func RunWithServer(runner RunnerFunc) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		opts := &PlexCtlOptions{
			OutputFormat: viper.GetString("output"),
			Verbosity:    viper.GetInt("verbose"),
			Sort:         viper.GetString("sort"),
		}

		client, err := plex.NewClient()
		if err != nil {
			return err
		}
		return runner(cmd.Context(), client, cmd, args, opts)
	}
}

// EnsureActiveServer checks if a server is configured and triggers discovery if not
func EnsureActiveServer(ctx context.Context) error {
	client, err := plex.NewClient()
	if err != nil {
		return err
	}

	if client.HasServer() {
		return nil
	}

	return DiscoverAndSelectServer(ctx)
}

// DiscoverAndSelectServer triggers the interactive server discovery flow
func DiscoverAndSelectServer(ctx context.Context) error {
	client, err := plex.NewClient()
	if err != nil {
		return err
	}

	res, err := client.SDK.Plex.GetServerResources(ctx, operations.GetServerResourcesRequest{
		IncludeHTTPS: operations.IncludeHTTPSTrue.ToPointer(),
		IncludeIPv6:  operations.IncludeIPv6True.ToPointer(),
		IncludeRelay: operations.IncludeRelayTrue.ToPointer(),
	})
	if err != nil {
		return fmt.Errorf("failed to discover servers: %w", err)
	}

	var options []struct{ Title, Desc, Value, ID string }
	for _, device := range res.PlexDevices {
		if strings.Contains(device.Provides, "server") {
			for _, conn := range device.Connections {
				if !conn.Relay {
					desc := conn.URI
					if conn.Local {
						desc += " (Local)"
					}
					options = append(options, struct{ Title, Desc, Value, ID string }{
						Title: device.Name,
						Desc:  desc,
						Value: conn.URI,
						ID:    device.ClientIdentifier,
					})
				}
			}
		}
	}

	if len(options) == 0 {
		return fmt.Errorf("no servers discovered on plex.tv")
	}

	// Adapt options for UI selector
	var uiOptions []struct{ Title, Desc, Value string }
	for _, o := range options {
		uiOptions = append(uiOptions, struct{ Title, Desc, Value string }{
			Title: o.Title,
			Desc:  o.Desc,
			Value: o.Value,
		})
	}

	choice, err := ui.SelectOption("Select a Plex Server to use", uiOptions)
	if err != nil {
		return fmt.Errorf("failed to select server: %w", err)
	}

	// Find the selected option to get the ID and Name
	var selectedID, selectedName string
	for _, opt := range options {
		if opt.Value == choice {
			selectedID = opt.ID
			selectedName = opt.Title
			break
		}
	}

	cfg := config.Get()
	cfg.AddServer(selectedID, config.Server{Name: selectedName, URL: choice}, true)
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	ui.RenderSuccess(fmt.Sprintf("Selected and saved server: %s (%s)", selectedName, choice))
	return nil
}

// Print formats and prints data using the provided Presenter
func Print(p presenters.Presenter, opts *PlexCtlOptions) error {
	sortCol := opts.Sort
	if sortCol == "" {
		sortCol = p.DefaultSort()
	}

	if sortCol != "" {
		p.SortBy(sortCol)
	}

	data := ui.OutputData{

		Title: p.Title(),

		Headers: p.Headers(),

		Rows: p.Rows(),

		Raw: p.Raw(),
	}

	return data.Print()
}

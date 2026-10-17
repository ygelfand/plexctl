package commands

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/presenters"
	"github.com/ygelfand/plexctl/internal/ui"
)

// RunnerFunc defines the signature for a command handler that receives a Plex client
type RunnerFunc func(ctx context.Context, client *plex.Client, cmd *cobra.Command, args []string, opts *PlexCtlOptions) error

// RunWithClient wraps a cobra command RunE function to inject a configured Plex client.
// This is for commands that only need a token (like login or discovery).
func RunWithClient(runner RunnerFunc) func(cmd *cobra.Command, args []string) error {
	return runWithClient(plex.NewAccountClient, runner)
}

func runWithClient(newClient func() (*plex.Client, error), runner RunnerFunc) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		opts := &PlexCtlOptions{
			OutputFormat: viper.GetString("output"),
			Verbosity:    viper.GetInt("verbose"),
			Sort:         viper.GetString("sort"),
			Count:        viper.GetInt("count"),
			Page:         viper.GetInt("page"),
			All:          viper.GetBool("all"),
		}

		client, err := newClient()
		if err != nil {
			return err
		}
		return runner(cmd.Context(), client, cmd, args, opts)
	}
}

// RunWithServer wraps RunWithClient but ensures a default server is selected first.
// This is for commands that interact with specific media or server library sections.
func RunWithServer(runner RunnerFunc) func(cmd *cobra.Command, args []string) error {
	return runWithClient(plex.NewClient, runner)
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
	servers, err := plex.DiscoverServers(ctx)
	if err != nil {
		return err
	}
	// Initial selection is for the user's own server. Shared servers are
	// discovered independently under Friends Libraries in the TUI.
	var owned []struct{ Title, Desc, Value string }
	for _, device := range servers {
		if device.Owned {
			owned = append(owned, struct{ Title, Desc, Value string }{device.Name, "Owned", device.ClientIdentifier})
		}
	}
	if len(owned) == 0 {
		return fmt.Errorf("no owned Plex servers discovered for this user")
	}
	choice := owned[0].Value
	if len(owned) > 1 {
		choice, err = ui.SelectOption("Select your Plex server", owned)
		if err != nil {
			return err
		}
	}
	for _, device := range servers {
		if device.ClientIdentifier != choice {
			continue
		}
		target, err := plex.ConnectServer(ctx, device)
		if err != nil {
			return err
		}
		cfg := config.Get()
		server := cfg.Servers[target.ID]
		server.Name, server.URL = target.Name, target.URL
		server.AccessToken, server.TokenSource = target.Token, cfg.TokenSource()
		server.Shared = target.Shared
		cfg.AddServer(target.ID, server, true)
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("save configuration: %w", err)
		}
		ui.RenderSuccess(fmt.Sprintf("Selected and saved server: %s", target.Name))
		return nil
	}
	return fmt.Errorf("selected server is no longer available")
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

	title := p.Title()
	// Suppress title for machine-readable formats
	if opts.OutputFormat != "table" && opts.OutputFormat != "text" && opts.OutputFormat != "txt" && opts.OutputFormat != "" {
		title = ""
	}

	data := ui.OutputData{
		Title:   title,
		Headers: p.Headers(),
		Rows:    p.Rows(),
		Raw:     p.Raw(),
	}

	return data.Print()
}

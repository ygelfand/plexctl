package cmd

import (
	"fmt"
	"strings"

	"github.com/BrenekH/go-plexauth"
	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/plex"
	"github.com/ygelfand/plexctl/internal/ui"
)

var dontOpen bool

var loginCmd = &cobra.Command{
	Use:     "login",
	Short:   "Login to Plex using a PIN flow",
	GroupID: "auth",
	Annotations: map[string]string{
		ui.AnnotationSkipServerCheck: "true",
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Get()
		appName := "plexctl"
		clientID := config.ClientIdentifier()
		theme := ui.CurrentTheme()

		pinID, pinCode, err := plexauth.GetPlexPIN(appName, clientID)
		if err != nil {
			return fmt.Errorf("failed to get Plex PIN: %w", err)
		}

		authURL, err := plexauth.GenerateAuthURL(appName, clientID, pinCode, plexauth.ExtraAuthURLOptions{})
		if err != nil {
			return fmt.Errorf("failed to generate auth URL: %w", err)
		}

		fmt.Println(ui.TitleStyle(theme).Render("Plex Authentication"))
		fmt.Printf("Please visit the following URL to authenticate:\n\n%s\n\n", ui.ValueStyle(theme).Underline(true).Render(authURL))
		fmt.Printf("%s %s\n", ui.LabelStyle(theme).Render("PIN Code:"), ui.ValueStyle(theme).Bold(true).Render(pinCode))

		if !dontOpen {
			fmt.Println("Opening browser...")
			if err := browser.OpenURL(authURL); err != nil {
				cfg.Logger.Warn("failed to open browser automatically", "error", err)
			}
		}

		fmt.Println("\nWaiting for authentication...")

		token, err := plexauth.PollForAuthToken(cmd.Context(), pinID, pinCode, clientID)
		if err != nil {
			return fmt.Errorf("failed to poll for token: %w", err)
		}

		cfg.Token = token
		ui.RenderSuccess("Successfully authenticated!")

		// Discover servers
		fmt.Println("\nDiscovering available servers...")
		client, err := plex.NewClient()
		if err != nil {
			return fmt.Errorf("failed to initialize client for discovery: %w", err)
		}

		res, err := client.SDK.Plex.GetServerResources(cmd.Context(), operations.GetServerResourcesRequest{
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
			cfg.Logger.Warn("No servers discovered on plex.tv")
		} else if len(options) == 1 {
			opt := options[0]
			cfg.AddServer(opt.ID, config.Server{Name: opt.Title, URL: opt.Value}, true)
			fmt.Printf("Auto-selected only available server: %s (%s)\n", opt.Title, opt.Value)
		} else {
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

			var selectedID, selectedName string
			for _, opt := range options {
				if opt.Value == choice {
					selectedID = opt.ID
					selectedName = opt.Title
					break
				}
			}

			cfg.AddServer(selectedID, config.Server{Name: selectedName, URL: choice}, true)
			fmt.Printf("Selected server: %s (%s)\n", selectedName, choice)
		}

		if err := cfg.Save(); err != nil {
			return fmt.Errorf("failed to save configuration: %w", err)
		}

		fmt.Printf("\nToken and configuration saved to %s\n", cfg.ConfigPath)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(loginCmd)

	loginCmd.Flags().BoolVar(&dontOpen, "dont-open", false, "Do not automatically open the browser")
}

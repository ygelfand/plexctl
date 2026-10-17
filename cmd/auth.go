package cmd

import (
	"fmt"

	"github.com/BrenekH/go-plexauth"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/ygelfand/plexctl/internal/commands"
	"github.com/ygelfand/plexctl/internal/config"
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

		cfg.HomeUser = config.HomeUser{}
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("save authentication: %w", err)
		}
		fmt.Println("\nDiscovering your Plex servers...")
		if err := commands.DiscoverAndSelectServer(cmd.Context()); err != nil {
			return err
		}

		fmt.Printf("\nToken and configuration saved to %s\n", cfg.ConfigPath)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(loginCmd)

	loginCmd.Flags().BoolVar(&dontOpen, "dont-open", false, "Do not automatically open the browser")
}

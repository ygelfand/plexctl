package cmd

import (
	"github.com/spf13/cobra"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/mpvsetup"
	"github.com/ygelfand/plexctl/internal/ui"
)

func checkMPV(cmd *cobra.Command, required, refresh, install, quiet bool) error {
	return mpvsetup.Ensure(cmd.Context(), mpvsetup.Options{
		Input: cmd.InOrStdin(), Output: cmd.ErrOrStderr(), Error: cmd.ErrOrStderr(),
		CacheDir: config.Get().CacheDir, Required: required, Refresh: refresh, Install: install, Quiet: quiet,
	})
}

func newMPVCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "mpv", Short: "Check the installed mpv and latest stable release", Args: cobra.NoArgs,
		GroupID: "media", Annotations: map[string]string{ui.AnnotationSkipServerCheck: "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return checkMPV(cmd, true, true, false, false)
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use: "install", Short: "Offer to install or update mpv with your approval", Args: cobra.NoArgs,
		Annotations: map[string]string{ui.AnnotationSkipServerCheck: "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return checkMPV(cmd, true, true, true, false)
		},
	})
	return cmd
}

func init() { rootCmd.AddCommand(newMPVCommand()) }

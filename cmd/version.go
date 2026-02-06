package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/ui"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version information",
	Annotations: map[string]string{
		ui.AnnotationSkipServerCheck: "true",
	},
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(config.FullVersion())
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

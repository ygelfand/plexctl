package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/ygelfand/plexctl/internal/commands"
	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/ui"
)

var (
	cfgFile    string
	verbosity  int
	sortCol    string
	noCache    bool
	outputType string
)

var rootCmd = &cobra.Command{
	Use:           "plexctl",
	Short:         "A robust CLI for managing your Plex Media Server",
	Version:       config.Version,
	Long:          `plexctl is a comprehensive command-line interface for interacting with Plex Media Server`,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip check if annotation is present
		if cmd.Annotations[ui.AnnotationSkipServerCheck] == "true" {
			return nil
		}
		// Skip for built-in help and completion
		if cmd.Name() == "help" || cmd.Name() == "completion" {
			return nil
		}
		return commands.EnsureActiveServer(cmd.Context())
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if config.Get().DefaultToTui {
			return runTUI()
		}
		cmd.Help()
		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		ui.RenderError(err)
		os.Exit(1)
	}
}

func GetRootCmd() *cobra.Command {
	return rootCmd
}

func init() {
	rootCmd.SetVersionTemplate(fmt.Sprintf("plexctl version {{.Version}} (commit: %s, date: %s)\n", config.GitCommit, config.BuildDate))
	cobra.OnInitialize(initConfig)

	// Define focused command groups
	rootCmd.AddGroup(&cobra.Group{ID: "tui", Title: "Interactive"})
	rootCmd.AddGroup(&cobra.Group{ID: "media", Title: "Media & Search"})
	rootCmd.AddGroup(&cobra.Group{ID: "auth", Title: "Authentication"})

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.plexctl.yaml)")
	rootCmd.PersistentFlags().StringVarP(&outputType, "output", "o", "table", "Output format (table, json, json-pretty, yaml, csv, txt)")
	rootCmd.PersistentFlags().CountP("verbose", "v", "increase verbosity")
	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))

	rootCmd.PersistentFlags().BoolVar(&noCache, "no-cache", false, "Disable caching")
	viper.BindPFlag("no_cache", rootCmd.PersistentFlags().Lookup("no-cache"))

	rootCmd.PersistentFlags().StringVar(&sortCol, "sort", "", "column to sort by")
	viper.BindPFlag("sort", rootCmd.PersistentFlags().Lookup("sort"))
}

func initConfig() {
	cfg := config.Get()
	cfg.Verbosity = viper.GetInt("verbose")
	cfg.SetupLogging()

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			ui.RenderError(fmt.Errorf("failed to get home directory: %w", err))
			os.Exit(1)
		}

		viper.AddConfigPath(home)
		viper.AddConfigPath(".")
		viper.SetConfigType("yaml")
		viper.SetConfigName(".plexctl")
	}

	viper.AutomaticEnv()
	viper.SetEnvPrefix("PLEXCTL")

	// Explicitly bind env vars that don't have corresponding flags
	viper.BindEnv("token")

	if err := viper.ReadInConfig(); err == nil {
		cfg.ConfigPath = viper.ConfigFileUsed()
	}

	// Unmarshal the loaded config into our struct
	if err := viper.Unmarshal(cfg); err != nil {
		ui.RenderError(fmt.Errorf("failed to parse config: %w", err))
		os.Exit(1)
	}

	// Ensure flags override config
	if outputType != "" && outputType != "table" {
		cfg.OutputFormat = outputType
	}
	if cfg.OutputFormat == "" {
		cfg.OutputFormat = "table"
	}

	// Basic validation for output format
	validFormats := map[string]bool{
		"table": true, "json": true, "json-pretty": true, "yaml": true, "csv": true, "txt": true, "text": true,
	}
	if !validFormats[cfg.OutputFormat] {
		ui.RenderError(fmt.Errorf("invalid output format: %s", cfg.OutputFormat))
		os.Exit(1)
	}

	if verbosity > 0 {
		cfg.Verbosity = verbosity
	}

	// Sync back to viper for parts that still use it
	viper.Set("output", cfg.OutputFormat)
}

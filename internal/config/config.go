package config

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/spf13/viper"
)

var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
)

// FullVersion returns a concatenated version string
func FullVersion() string {
	return fmt.Sprintf("%s-%s-%s", Version, GitCommit, BuildDate)
}

// ClientIdentifier returns the standard identifier used for Plex requests
func ClientIdentifier() string {
	return "plexcli"
}

// GetHostname returns the current machine's hostname
func GetHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

type IconType string

const (
	IconTypeASCII     IconType = "ascii"
	IconTypeEmoji     IconType = "emoji"
	IconTypeNerdFonts IconType = "nerdfonts"
)

type LibraryNameFormat string

const (
	LibraryNameFormatIconOnly LibraryNameFormat = "icon_only"
	LibraryNameFormatIconName LibraryNameFormat = "icon_name"
	LibraryNameFormatNameIcon LibraryNameFormat = "name_icon"
	LibraryNameFormatName     LibraryNameFormat = "name"
)

type ViewMode string

const (
	ViewModeList   ViewMode = "list"
	ViewModePoster ViewMode = "poster"
)

type LibraryOptions struct {
	IconEmoji string   `mapstructure:"icon_emoji" yaml:"icon_emoji"`
	IconNF    string   `mapstructure:"icon_nf" yaml:"icon_nf"`
	IconASCII string   `mapstructure:"icon_ascii" yaml:"icon_ascii"`
	Hidden    bool     `mapstructure:"hidden" yaml:"hidden"`
	ViewMode  ViewMode `mapstructure:"view_mode" yaml:"view_mode"`
}

func (o LibraryOptions) GetIcon(t IconType) string {
	switch t {
	case IconTypeEmoji:
		return o.IconEmoji
	case IconTypeNerdFonts:
		return o.IconNF
	case IconTypeASCII:
		return o.IconASCII
	default:
		return ""
	}
}

type LibraryConfig struct {
	Order    []string                  `mapstructure:"order" yaml:"order"`
	Settings map[string]LibraryOptions `mapstructure:"settings" yaml:"settings"`
}

type Server struct {
	Name      string        `mapstructure:"name" yaml:"name"`
	URL       string        `mapstructure:"url" yaml:"url"`
	Libraries LibraryConfig `mapstructure:"libraries" yaml:"libraries"`
}

type HomeUser struct {
	AuthToken   string `mapstructure:"auth_token" yaml:"auth_token"`     // V2 Switch User Token
	AccessToken string `mapstructure:"access_token" yaml:"access_token"` // Server-specific Access Token
}

// Config holds the global configuration for plexctl
type Config struct {
	// Global settings
	Token             string            `mapstructure:"token"`
	HomeUser          HomeUser          `mapstructure:"home_user"`
	OutputFormat      string            `mapstructure:"output"`
	Verbosity         int               `mapstructure:"verbose"`
	Theme             string            `mapstructure:"theme"`
	IconType          IconType          `mapstructure:"icon_type"`           // ascii, emoji, nerdfonts
	LibraryNameFormat LibraryNameFormat `mapstructure:"library_name_format"` // icon_only, icon_name, name_icon, name
	DefaultViewMode   ViewMode          `mapstructure:"default_view_mode"`   // list, poster
	CacheDir          string            `mapstructure:"cache_dir"`
	NoCache           bool              `mapstructure:"no_cache"`
	DefaultToTui      bool              `mapstructure:"default_to_tui"`
	AutoHomeLogin     bool              `mapstructure:"auto_home_login"`
	CloseVideoOnQuit  bool              `mapstructure:"close_video_on_quit"`

	// Server management
	DefaultServer string            `mapstructure:"default_server"` // Stores the ClientIdentifier
	Servers       map[string]Server `mapstructure:"servers"`        // Key is ClientIdentifier

	// Runtime only
	ConfigPath string       `mapstructure:"-"`
	LogFile    string       `mapstructure:"-"`
	Logger     *slog.Logger `mapstructure:"-"`
	LogLevel   *slog.LevelVar
}

var (
	instance *Config
	once     sync.Once
)

const (
	LevelTrace slog.Level = -8
)

// Get returns the global configuration singleton
func Get() *Config {
	once.Do(func() {
		home, _ := os.UserHomeDir()
		lvl := &slog.LevelVar{}
		lvl.Set(slog.LevelInfo)
		instance = &Config{
			Logger:          slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})),
			LogLevel:        lvl,
			Servers:         make(map[string]Server),
			CacheDir:        filepath.Join(home, ".plexctl", "cache"),
			DefaultToTui:    true,
			AutoHomeLogin:   true,
			DefaultViewMode: ViewModePoster,
		}
	})
	return instance
}

// SetupLogging initializes the global logger based on verbosity
func (c *Config) SetupLogging() {
	var level slog.Level
	switch {
	case c.Verbosity >= 2:
		level = LevelTrace
	case c.Verbosity >= 1:
		level = slog.LevelDebug
	default:
		level = slog.LevelInfo
	}

	c.LogLevel.Set(level)

	opts := &slog.HandlerOptions{
		Level: c.LogLevel,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.LevelKey {
				level := a.Value.Any().(slog.Level)
				if level == LevelTrace {
					a.Value = slog.StringValue("TRACE")
				}
			}
			return a
		},
	}

	var writer io.Writer = os.Stderr
	if c.LogFile != "" {
		_ = os.MkdirAll(filepath.Dir(c.LogFile), 0755)
		f, err := os.OpenFile(c.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			writer = f
		}
	}

	handler := slog.NewTextHandler(writer, opts)
	c.Logger = slog.New(handler)
	slog.SetDefault(c.Logger)
}

// Enabled returns true if the given level is enabled
func (c *Config) Enabled(level slog.Level) bool {
	return c.LogLevel.Level() <= level
}

// Save persists the current configuration to disk
func (c *Config) Save() error {
	// Sync struct fields to viper before writing
	viper.Set("token", c.Token)
	viper.Set("home_user", c.HomeUser)
	viper.Set("output", c.OutputFormat)
	viper.Set("verbose", c.Verbosity)
	viper.Set("theme", c.Theme)
	viper.Set("icon_type", c.IconType)
	viper.Set("library_name_format", c.LibraryNameFormat)
	viper.Set("default_view_mode", c.DefaultViewMode)
	viper.Set("auto_home_login", c.AutoHomeLogin)
	viper.Set("close_video_on_quit", c.CloseVideoOnQuit)
	viper.Set("cache_dir", c.CacheDir)
	viper.Set("default_server", c.DefaultServer)
	viper.Set("servers", c.Servers)

	if c.ConfigPath != "" {
		return viper.WriteConfig()
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	c.ConfigPath = filepath.Join(home, ".plexctl.yaml")
	return viper.WriteConfigAs(c.ConfigPath)
}

// GetActiveServer returns the configuration for the currently selected server.
// It prioritizes the server ID in 'default_server'.
// Returns the server ID, config, and a boolean indicating if found.
func (c *Config) GetActiveServer() (string, Server, bool) {
	if c.DefaultServer == "" {
		return "", Server{}, false
	}
	srv, ok := c.Servers[c.DefaultServer]
	return c.DefaultServer, srv, ok
}

// SetDefaultServer sets the default server by ID or Name
func (c *Config) SetDefaultServer(idOrName string) error {
	// Try ID first
	if _, ok := c.Servers[idOrName]; ok {
		c.DefaultServer = idOrName
		return nil
	}

	// Try Name
	for id, srv := range c.Servers {
		if srv.Name == idOrName {
			c.DefaultServer = id
			return nil
		}
	}

	return fmt.Errorf("server '%s' not found in configuration", idOrName)
}

// AddServer adds or updates a server configuration and optionally sets it as default
func (c *Config) AddServer(id string, config Server, setAsDefault bool) {
	c.Servers[id] = config
	if setAsDefault {
		c.DefaultServer = id
	}
}

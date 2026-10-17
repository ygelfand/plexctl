package config

import (
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestParseTerminalVideo(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"auto", "kitty", "sixel", "tct", "", "gpu", "kitty,sixel", "--vo=gpu"} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			got, err := ParseTerminalVideo(value)
			valid := value == "auto" || value == "kitty" || value == "sixel" || value == "tct"
			if valid && (err != nil || string(got) != value) {
				t.Fatalf("ParseTerminalVideo(%q) = %q, %v", value, got, err)
			}
			if !valid && err == nil {
				t.Fatalf("accepted invalid renderer %q", value)
			}
		})
	}
}

func TestPlaybackSettingsSurviveSaveAndReload(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	path := filepath.Join(t.TempDir(), "plexctl.yaml")
	viper.SetConfigFile(path)
	settings := Config{
		ConfigPath: path, TerminalVideo: VideoSixel, TerminalVideoMode: "overlay",
		SixelVideoWidth: 480, PosterRenderer: "halfcell", WindowMusicVisualizer: "scope",
		MusicPlayer: "window", MusicVisualizer: "mirror",
	}
	if err := settings.Save(); err != nil {
		t.Fatal(err)
	}
	loaded := viper.New()
	loaded.SetConfigFile(path)
	if err := loaded.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	var got Config
	if err := loaded.Unmarshal(&got); err != nil {
		t.Fatal(err)
	}
	if err := got.ValidatePlayback(); err != nil {
		t.Fatal(err)
	}
	if got.TerminalVideo != VideoSixel || got.TerminalVideoMode != "overlay" ||
		got.SixelVideoWidth != 480 || got.PosterRenderer != "halfcell" ||
		got.WindowMusicVisualizer != "scope" || got.MusicPlayer != "window" || got.MusicVisualizer != "mirror" {
		t.Fatal("saved playback preferences changed after reloading configuration")
	}
	for _, tt := range []struct {
		name string
		edit func(*Config)
	}{
		{"negative pixel cap", func(c *Config) { c.SixelVideoWidth = -1 }},
		{"unknown poster protocol", func(c *Config) { c.PosterRenderer = "gpu" }},
		{"unknown video mode", func(c *Config) { c.TerminalVideoMode = "background" }},
		{"unsupported music graph", func(c *Config) { c.WindowMusicVisualizer = "invalid" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			invalid := got
			tt.edit(&invalid)
			if invalid.ValidatePlayback() == nil {
				t.Fatal("invalid playback setting accepted")
			}
		})
	}
}

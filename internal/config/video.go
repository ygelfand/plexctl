package config

import "fmt"

// VideoOutput selects an mpv video renderer. The zero value opens a video window.
type VideoOutput string

const (
	VideoWindow VideoOutput = ""
	VideoAuto   VideoOutput = "auto"
	VideoKitty  VideoOutput = "kitty"
	VideoSixel  VideoOutput = "sixel"
	VideoTCT    VideoOutput = "tct"
	// AudioTerminal keeps the TUI active and disables mpv's video window.
	AudioTerminal VideoOutput = "audio"
)

// ParseTerminalVideo validates a terminal renderer preference.
func ParseTerminalVideo(value string) (VideoOutput, error) {
	switch output := VideoOutput(value); output {
	case VideoAuto, VideoKitty, VideoSixel, VideoTCT:
		return output, nil
	default:
		return VideoWindow, fmt.Errorf("invalid terminal video renderer %q: choose auto, kitty, sixel, or tct", value)
	}
}

// ValidatePlayback checks settings before any renderer or player is started.
func (c *Config) ValidatePlayback() error {
	if _, err := ParseTerminalVideo(string(c.TerminalVideo)); err != nil {
		return err
	}
	for _, setting := range []struct {
		name, value string
		allowed     []string
	}{
		{name: "poster_renderer", value: c.PosterRenderer, allowed: []string{"auto", "kitty", "sixel", "halfcell"}},
		{name: "terminal_video_mode", value: c.TerminalVideoMode, allowed: []string{"suspend", "overlay"}},
		{name: "window_music_visualizer", value: c.WindowMusicVisualizer, allowed: []string{"cqt", "spectrum", "waves", "scope", "off"}},
	} {
		valid := false
		for _, choice := range setting.allowed {
			valid = valid || setting.value == choice
		}
		if !valid {
			return fmt.Errorf("invalid %s %q: choose %v", setting.name, setting.value, setting.allowed)
		}
	}
	if c.SixelVideoWidth < 0 {
		return fmt.Errorf("sixel_video_width must be zero (automatic) or a positive pixel width")
	}
	return nil
}

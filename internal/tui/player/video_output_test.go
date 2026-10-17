package player

import (
	"slices"
	"testing"

	"github.com/ygelfand/plexctl/internal/config"
	"github.com/ygelfand/plexctl/internal/termgraphics"
)

func TestKittySharedMemoryRequiresSuccessfulProbe(t *testing.T) {
	for _, local := range []bool{false, true} {
		p := terminalPlayback{output: config.VideoKitty, sharedMemory: local}
		want, unwanted := "--vo-kitty-use-shm=no", "--vo-kitty-use-shm=yes"
		if local {
			want, unwanted = unwanted, want
		}
		args := p.args("ipc")
		if !slices.Contains(args, want) || slices.Contains(args, unwanted) {
			t.Fatalf("sharedMemory=%v: unsafe transport arguments %v", local, args)
		}
		if !slices.Contains(args, "--profile=sw-fast") {
			t.Fatal("missing software-rendering performance profile")
		}
	}
}

func TestSelectTerminalOutput(t *testing.T) {
	t.Parallel()
	const all = "Available video outputs:\n  kitty Kitty graphics\n  sixel Sixel graphics\n  tct true-color terminals\n"
	tests := []struct {
		name       string
		preference config.VideoOutput
		help       string
		env        map[string]string
		caps       termgraphics.Capabilities
		want       config.VideoOutput
		wantErr    bool
	}{
		{name: "kitty", preference: config.VideoAuto, help: all, env: map[string]string{"TERM": "xterm-kitty"}, want: config.VideoKitty},
		{name: "kitty over ssh", preference: config.VideoAuto, help: all, env: map[string]string{"TERM": "xterm-kitty", "SSH_CONNECTION": "remote"}, want: config.VideoKitty},
		{name: "ghostty", preference: config.VideoAuto, help: all, env: map[string]string{"TERM_PROGRAM": "ghostty"}, want: config.VideoKitty},
		{name: "sixel", preference: config.VideoAuto, help: all, env: map[string]string{"TERM": "foot"}, want: config.VideoSixel},
		{name: "wezterm", preference: config.VideoAuto, help: all, env: map[string]string{"TERM_PROGRAM": "WezTerm"}, want: config.VideoSixel},
		{name: "konsole without kitty driver", preference: config.VideoAuto, help: "sixel graphics\ntct text", env: map[string]string{"KONSOLE_VERSION": "250000"}, want: config.VideoSixel},
		{name: "unknown terminal", preference: config.VideoAuto, help: all, env: map[string]string{"TERM": "xterm-256color"}, want: config.VideoTCT},
		{name: "missing graphics driver", preference: config.VideoAuto, help: "sixel graphics\ntct text", env: map[string]string{"TERM": "xterm-kitty"}, want: config.VideoTCT},
		{name: "tmux hides outer kitty", preference: config.VideoAuto, help: all, env: map[string]string{"TMUX": "socket", "KITTY_WINDOW_ID": "1"}, want: config.VideoTCT},
		{name: "probed kitty through tmux without tct", preference: config.VideoAuto, help: "kitty graphics", env: map[string]string{"TMUX": "socket"}, caps: termgraphics.Capabilities{Responded: true, Kitty: true}, want: config.VideoKitty},
		{name: "probed sixel through tmux", preference: config.VideoAuto, help: all, env: map[string]string{"TMUX": "socket"}, caps: termgraphics.Capabilities{Responded: true, Sixel: true}, want: config.VideoSixel},
		{name: "probe overrides stale kitty environment", preference: config.VideoAuto, help: all, env: map[string]string{"TERM": "xterm-kitty"}, caps: termgraphics.Capabilities{Responded: true, Name: "VTE(7600)"}, want: config.VideoTCT},
		{name: "screen hides outer kitty", preference: config.VideoAuto, help: all, env: map[string]string{"TERM": "screen-256color", "KITTY_WINDOW_ID": "1"}, want: config.VideoTCT},
		{name: "explicit sixel", preference: config.VideoSixel, help: all, want: config.VideoSixel},
		{name: "explicit kitty in tmux", preference: config.VideoKitty, help: all, env: map[string]string{"TMUX": "socket"}, want: config.VideoKitty},
		{name: "explicit tct", preference: config.VideoTCT, help: all, env: map[string]string{"TERM": "xterm-kitty"}, want: config.VideoTCT},
		{name: "explicit missing renderer", preference: config.VideoKitty, help: "tct text", wantErr: true},
		{name: "do not match descriptions", preference: config.VideoKitty, help: "gpu kitty sixel tct", wantErr: true},
		{name: "no compatible renderer", preference: config.VideoAuto, help: "gpu window\nkitty graphics", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := selectTerminalOutput(tt.preference, tt.help, func(key string) string { return tt.env[key] }, tt.caps)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, want error %v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

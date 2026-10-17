## plexctl session

Manage active playback sessions

### Options

```
  -h, --help   help for session
```

### Options inherited from parent commands

```
      --config string                    config file (default is $HOME/.plexctl.yaml)
      --no-cache                         Disable caching
  -o, --output string                    Output format (table, json, json-pretty, yaml, csv, txt) (default "table")
      --poster-renderer string           Poster renderer (auto, kitty, sixel, halfcell) (default "auto")
      --sixel-video-width int            Maximum Sixel video width in pixels (0 uses terminal size) (default 640)
      --sort string                      column to sort by
      --terminal-video string            Terminal video renderer for Ctrl+P or play (auto, kitty, sixel, tct) (default "auto")
      --terminal-video-mode string       TUI terminal video mode (suspend, overlay) (default "suspend")
  -v, --verbose count                    increase verbosity
      --window-music-visualizer string   Music video visualization (cqt, spectrum, waves, scope, off) (default "cqt")
```

### SEE ALSO

* [plexctl](plexctl.md)	 - A robust CLI for managing your Plex Media Server
* [plexctl session list](plexctl_session_list.md)	 - List all active playback sessions
* [plexctl session show](plexctl_session_show.md)	 - Show detailed information for a playback session
* [plexctl session stop](plexctl_session_stop.md)	 - Terminate an active playback session


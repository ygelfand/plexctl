## plexctl

A robust CLI for managing your Plex Media Server

### Synopsis

plexctl is a comprehensive command-line interface for interacting with Plex Media Server

```
plexctl [flags]
```

### Options

```
      --config string                    config file (default is $HOME/.plexctl.yaml)
  -h, --help                             help for plexctl
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

* [plexctl collection](plexctl_collection.md)	 - Manage collections
* [plexctl device](plexctl_device.md)	 - Manage account devices
* [plexctl history](plexctl_history.md)	 - Show playback history
* [plexctl homeusers](plexctl_homeusers.md)	 - Manage Plex Home users
* [plexctl hub](plexctl_hub.md)	 - Manage hubs
* [plexctl library](plexctl_library.md)	 - Manage libraries
* [plexctl login](plexctl_login.md)	 - Login to Plex using a PIN flow
* [plexctl mpv](plexctl_mpv.md)	 - Check the installed mpv and latest stable release
* [plexctl play](plexctl_play.md)	 - Play a media item
* [plexctl playlist](plexctl_playlist.md)	 - Manage playlists
* [plexctl search](plexctl_search.md)	 - Manage and use the library search index
* [plexctl server](plexctl_server.md)	 - Manage Plex Server
* [plexctl session](plexctl_session.md)	 - Manage active playback sessions
* [plexctl tasks](plexctl_tasks.md)	 - Manage background butler tasks
* [plexctl tui](plexctl_tui.md)	 - Launch the interactive TUI
* [plexctl users](plexctl_users.md)	 - Manage Plex users
* [plexctl version](plexctl_version.md)	 - Print the version information


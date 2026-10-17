## plexctl tasks

Manage background butler tasks

### Options

```
  -h, --help   help for tasks
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
* [plexctl tasks list](plexctl_tasks_list.md)	 - List all butler tasks
* [plexctl tasks start](plexctl_tasks_start.md)	 - Start a butler task
* [plexctl tasks stop](plexctl_tasks_stop.md)	 - Stop a butler task


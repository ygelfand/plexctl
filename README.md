# plexctl

A robust Plex CLI and TUI written in Go.

> **Note**: This project is a **Work in Progress**. Expect breaking changes and incomplete features.

![plexctl demo](assets/demo.gif)

## Prerequisites

`plexctl` uses **mpv** for media playback. You must have it installed and available in your PATH.

### Installation Instructions

- **macOS**: `brew install mpv`
- **Linux**: `sudo apt install mpv` or `sudo pacman -S mpv`
- **Windows**: Download from [mpv.io](https://mpv.io/)

## Installation

Download the latest binary for your platform from the [Releases](https://github.com/ygelfand/plexctl/releases) page.

### Manual Installation (macOS/Linux)

If you download the binary directly, you will need to make it executable and move it to your path:

```bash
# Rename the downloaded binary (example for macOS ARM)
mv plexctl_Darwin_arm64 plexctl

# Make it executable
chmod +x plexctl

# Move to a directory in your PATH (optional)
sudo mv plexctl /usr/local/bin/
```

Alternatively, if you have Go installed:

```bash
go install github.com/ygelfand/plexctl@latest
```

## Features

- **Interactive TUI**: Navigate your Plex library with ease.
- **Fuzzy Search**: Find exactly what you're looking for instantly.
- **Cross-Platform**: Supports macOS, Linux, and Windows.
- **Flexible Output**: CLI commands support table, JSON, YAML, and CSV formats.

## Configuration

`plexctl` will guide you through the login and server discovery process on first run.

```bash
# Start the TUI
plexctl

# Or use the CLI
plexctl login
plexctl search find "Inception"
```

## License

MIT

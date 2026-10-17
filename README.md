# plexctl

A robust Plex CLI and TUI written in Go.

> **Note**: This project is a **Work in Progress**. Expect breaking changes and incomplete features.

![plexctl demo](assets/demo.gif)

## Prerequisites

`plexctl` uses **mpv** for media playback. Before opening the TUI or playing media,
it checks the copy on your PATH against the latest stable mpv release. When mpv
is missing or outdated, it offers installation or an update and shows the exact
installation commands. Nothing is installed unless you answer **yes** in an
interactive terminal. You can decline and keep browsing or use an existing player.

Run `plexctl mpv` to check explicitly, or `plexctl mpv install` to approve an
installation/update. These commands do not require a Plex login. Release checks
are cached for a day; offline checks do not block an installed player. Development
builds newer than the stable release are not downgraded.

Automatic installation supports Homebrew and common Linux package managers.
On Debian/Ubuntu, plexctl checks apt's candidate first. If it is older than the
latest stable release, it offers to compile that release from source using
[mpv's Meson build](https://github.com/mpv-player/mpv#compilation). The dependency
pre-check lists all required packages and identifies which need installing or
upgrading, including **Meson from apt**, FFmpeg development libraries, libass,
Lua, audio/display headers, and **libsixel-dev**. You approve the dependency and
build commands together. An unavailable or outdated libplacebo is built as a
static Meson subproject. Older distributions that cannot supply the required
Meson/FFmpeg development versions receive an actionable pre-check error.

Source builds enable Sixel and Lua explicitly, verify Kitty/Sixel/text/window
video outputs, and install under `~/.local/opt/plexctl-mpv-<version>-<build-id>`.
Only a successful build is activated via `~/.local/bin/mpv`; an existing file or
symlink is preserved alongside it. Put `~/.local/bin` first on your PATH for future
shells (plexctl prints the command when needed). Build logs and sources remain in
the plexctl cache for troubleshooting. A failed build keeps the previous player.
Other package managers recheck the installed version and report outdated packages;
unsupported systems receive instructions from [mpv.io](https://mpv.io/installation/).

### Installation Instructions

- **macOS**: `brew install mpv`
- **Debian/Ubuntu**: `plexctl mpv install` (uses a source build when apt is too old)
- **Other Linux**: Use your package manager, such as `sudo pacman -S mpv`
- **Windows**: Download from [mpv.io](https://mpv.io/)

## Installation

Download the latest binary for your platform from the [Releases](https://github.com/ygelfand/plexctl/releases) page.

### Manual Installation (macOS/Linux)

If you download the binary directly, you will need to make it executable and move it to your path:

```bash
# Rename the downloaded binary (example for macOS ARM)
mv plexctl_darwin_arm64 plexctl

# Make it executable
chmod +x plexctl

# Move to a directory in your PATH (optional)
sudo mv plexctl /usr/local/bin/
```

Alternatively, with Go **1.27.2 or newer** installed:

```bash
go install github.com/ygelfand/plexctl@latest
```

Builds made with Go 1.27 require macOS 13 Ventura or newer on macOS.

## Development checks

Use Go 1.27.2 or newer and golangci-lint 2.14.0, built with the same Go version:

```bash
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0
go build ./...
go test -race -tags=integration ./...
golangci-lint run
golangci-lint run --build-tags integration
```

Player integration tests use synthetic media and a local mpv installation, and
skip when mpv is unavailable. `PLEXCTL_TEST_MPV=/path/to/mpv` selects an alternate
build for the overlay compatibility test.
Pull requests run these checks and `govulncheck` in GitHub Actions.

## Features

- **Interactive TUI**: Navigate your Plex library with ease.
- **Friends Libraries**: Browse and search libraries shared with your Plex account from the sidebar.
- **Fuzzy Search**: Find exactly what you're looking for instantly.
- **Cross-Platform**: Supports macOS, Linux, and Windows.
- **Flexible Output**: CLI commands support table, JSON, YAML, and CSV formats.
- **Complete music queues**: Play an artist or album with per-track progress,
  including soundtrack and compilation contributions.
- **Terminal graphics**: Kitty/Ghostty and Sixel posters, suspend/overlay video,
  and separate music visualizers for the TUI and mpv window.

## Usage

### TUI (Interactive)
Simply run `plexctl` to launch the interactive terminal interface.

Choose your own Plex server during setup. **Friends Libraries → Browse** in the
left sidebar discovers servers shared with your current Plex user. Open a friend,
then a library to use the same poster/list views, details, and playback controls.
Press **/** to search that friend's server, **Esc** to go back, and **r** on the
friends list to refresh invitations. Your own library tabs remain available.

Press **v** on a movie or episode to choose **Kitty**, **Sixel**, or **Text**
playback. **Ctrl+P** starts terminal playback using your saved preference.
Kitty video includes a visible control bar: play/pause, a draggable seek bar,
skip backward/forward, volume, mute, subtitles, playback speed, and **Return**.
Click the controls or use **Tab / Shift+Tab** and **Enter**; **h** hides or shows
the bar. In tmux, playback stays within the current pane. Press **f** or click
**Fullscreen** to zoom it; press **f** again or click **Restore** to return to
the split layout. The controls also work through tmux and SSH into a Kitty terminal.

The player uses mpv's Kitty or Sixel graphics output when detected, with TCT text
output as a fallback. Press **Space** to pause, use the arrow keys to seek, and
press **q** to return to the library. Regular **p** opens the video window.

Choose a renderer in **Ctrl+S → Terminal Video**, or override it at launch:

```bash
plexctl tui --terminal-video kitty
plexctl tui --terminal-video sixel
plexctl play 12345 --terminal-video auto
```

Graphics playback requires a compatible terminal and an mpv build that lists the
chosen renderer in `mpv --vo=help`. plexctl probes the attached terminal, including
through tmux, and uses text output when graphics are unavailable. In tmux 3.3+,
suspended playback preserves the pane size until you toggle fullscreen with **f**.
Playback restores the original zoom state on exit if you used this control.
On Linux, a terminal that confirms local shared-memory access uses that faster
transport; SSH and other terminals keep using portable graphics packets.

**Ctrl+S → Terminal Video Mode → Overlay** keeps the TUI and its player controls
active while video draws over the content pane. Use **Space**, **[ / ]**,
**+ / -**, **m**, and **x** to control playback. Sixel video defaults to a centered
640-pixel width cap; change it in Settings or with `--sixel-video-width`.

Mouse support is enabled automatically: click posters, navigation, and dialog
options; scroll with the wheel; right click to go back. During terminal video,
click to pause, scroll to seek, and right click to return. **V** switches library
list/poster views.

**Ctrl+H/J/K/L** moves focus left/down/up/right between the sidebar, library,
alphabet rail, and active player. The focused pane is highlighted. In the sidebar,
use **j/k** or the arrows to select a library, then **Enter** to open it.
**L** opens library settings; **Ctrl+S** opens global settings.
In Sessions, History, and Tasks, **j/k** select rows and **h/l** scroll wide tables
left/right. The search Status page scrolls with **hjkl** too, including during
indexing. Arrow keys provide the same navigation.

Movie and TV cover posters automatically use native images in Kitty 0.28+,
Ghostty, and terminals advertising Sixel (also through tmux), with half-cell
text as a fallback. **Ctrl+S → Poster Renderer** overrides automatic detection.
See the [TUI guide](docs/content/tui.mdx) for configuration and playback details.

Music libraries support artist → album → track navigation. Press **p** to play
the selected song in the TUI, with a live stereo waveform and no extra
window. **v** chooses terminal audio or a separate window; **Ctrl+S → Music
Player** saves the default. Album covers appear in the artist gallery and beside
the track list. Scrolling automatically loads more artists. Playing an artist or
album queues all its available tracks, updating the title and reporting each
track to Plex. Artist browsing includes soundtrack and compilation albums.
Windowed music offers **cqt**, **spectrum**, **waves**, and **scope** visualizations
under **Ctrl+S → Music Video Visualizer**, plus **off**.

Click the right-hand **A–Z rail**, or press **g then a letter**, to jump through
movies, TV shows, artists, albums, and track titles. Missing library pages load
automatically. From the library, **Ctrl+L** focuses the rail: **j/k** or **Down/Up**
select a letter, and **Enter** jumps to it and returns focus to the content.
**Ctrl+H** or **Esc** returns without jumping. In the content pane,
**Home / End** reach the first/last item; **Page Up / Page Down**
move by a page. The library footer shows how many items have loaded.

Use **Space** to pause, **[ / ]** to seek, **+ / -** for volume, **m** to mute,
**z** to cycle waveform/spectrum/mirror/off, and **x** to stop. The default
waveform draws the playing audio with cyan/violet traces and fading trails.
It uses ordinary terminal cells, including through tmux and
SSH. **/** searches Plex for artists, albums, and track names without requiring
a local index first.

### CLI (Command Line)
`plexctl` provides a comprehensive command-line interface for automation and quick tasks. Every command supports the `-h` or `--help` flag for detailed usage instructions.

```bash
# Explore available commands
plexctl -h

# List all libraries on your server
plexctl library list

# Perform a fuzzy search
plexctl search find "Inception"

# Manage active playback sessions
plexctl session list

# Start or stop background server tasks
plexctl tasks list
```

All CLI commands support the `-o` or `--output` flag to return data in `table`, `json`, `json-pretty`, `yaml`, `csv`, or `txt` formats.

## Configuration

### Plex token (`PLEXCTL_TOKEN`)

Set `PLEXCTL_TOKEN` to authenticate with Plex using an environment variable. It
takes precedence over the account token saved in your plexctl configuration.

To find your token in the browser:

1. Open [Plex Web](https://app.plex.tv/) and sign in to your Plex account.
2. Pick any movie or show from your own server. For a show, open an episode if
   needed.
3. Open the **three-dot menu (⋯)** and choose **Get Info** near the bottom.
4. Click **View XML** at the bottom of the information window.
5. In the new browser tab, look near the end of the URL for `X-Plex-Token=`.
   Copy only the value after `=`, stopping before the next `&` if present. Do not
   include `X-Plex-Token=` or the rest of the URL.

For example, if the URL ends with `?X-Plex-Token=YOUR_TOKEN`, copy `YOUR_TOKEN`.
The XML link is available when signed in as the server owner; see
[Plex's media information guide](https://support.plex.tv/articles/202462186-viewing-item-details/).

On macOS or Linux with Bash or Zsh, replace the placeholder with your token:

```bash
export PLEXCTL_TOKEN='YOUR_TOKEN'
plexctl server discover
plexctl
```

The export applies to the current terminal session. To keep it for future
sessions, add the same `export` line to `~/.zshrc` for Zsh or `~/.bashrc` for Bash,
then open a new terminal.

On Windows with PowerShell, set it for the current session with:

```powershell
$env:PLEXCTL_TOKEN = 'YOUR_TOKEN'
plexctl server discover
plexctl
```

Keep your token private. Plex describes tokens obtained this way as temporary;
if authentication stops working, retrieve a fresh token using the same steps.
See [Plex's token guide](https://support.plex.tv/articles/204059436-finding-an-authentication-token-x-plex-token/)
for more details.

## License

MIT

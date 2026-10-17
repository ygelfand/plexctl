# Fork integration plan

Source: `/home/phil/plexctl-fork-comparison.md`, verified against
`brijohn-plexctl` at `c734407` and this repository at `8e6f099`.
The report calls the friend's fork “ours” and this fork “Astra”.

## Scope and decisions

- Keep this fork's multi-server credentials/cache isolation, mouse and keyboard
  navigation, tmux transport, terminal controls, audio visualizer, and CI.
- Adopt complete artist/album queues and per-track titles and Plex timeline
  reporting in both CLI and TUI playback. Preserve track order, pagination,
  cancellation, no-report playback, and the originating server.
- Include soundtrack/compilation albums and tracks with section `artist.id`
  queries, with children traversal when section metadata is unavailable.
- Introduce a poster renderer interface and capability-ordered registry. Keep
  the existing bounded Kitty upload cache and renderer-owned output; add
  Ghostty placeholder detection, explicit renderer selection, aspect-aware
  sizing, and Sixel posters with clipping and duplicate placements.
- Add selectable suspended/overlay terminal video, a centered Sixel pixel-width
  cap, and fallback to text when automatic graphics playback fails at startup.
- Add separate window/terminal-video music visualizer settings (CQT, spectrum,
  waves, scope, off), preserving the existing in-TUI visualizer settings.
- Document all new and existing playback/rendering settings and shortcuts.

## Evidence corrections

The supplied friend's checkout has no Sixel poster renderer or Sixel width
setting, despite the report describing them. Implement these directly as useful
gaps identified by the comparison. Album covers already exist here. Avoid
porting the friend's unsynchronized player state, authenticated-URL logging,
environment-only terminal probe, or fixed-version mpv argument guesses.

## Implementation sequence

1. Add paginated artist queries and queue resolution; wire browsing and playback.
2. Track playlist transitions and report each track, including final completion;
   cover both persistent and foreground players with local IPC/HTTP tests.
3. Add renderer selection and Sixel composition; verify clipping, cleanup,
   duplicate images, and existing Kitty cache behavior.
4. Add overlay playback, Sixel sizing, visualizer graphs, and settings/CLI wiring.
5. Update docs and run formatting, race/shuffle tests, real-mpv integration tests,
   lint, builds, module consistency, and the vulnerability check.

## Validation boundaries

Tests use local fixtures and synthetic media, without real Plex credentials.
Live appearance and performance across physical Kitty, Ghostty, Sixel terminals,
and tmux require interactive acceptance testing on those terminals.

## Progress

- All five implementation steps are complete. Configuration, TUI Settings, CLI
  flags, generated command pages, README, and the config/TUI guides are updated.
- Album/artist queues retain playlist-entry identity, resume only the first
  track, preserve external subtitles, and report individual completions in
  order. Stop/new selection cancels pending resolution. Regression coverage
  includes 20 ms tracks, repeated file paths, reconnect, events during IPC
  sampling, delayed HTTP, and bounded shutdown. Foreground queues drain IPC
  acknowledgements as they are built, avoiding backpressure on large queues.
- Poster renderers preserve the existing Kitty disk cache and bounded terminal
  textures. Sixel tests cover duplicate placements, clipped rows/columns,
  partial six-pixel bands, dialogs, unchanged frames, and cleanup. Capability
  tests cover Ghostty and terminal cell dimensions.
- Kitty, Sixel, and Text overlays were exercised with real mpv and synthetic
  audio visualization, including pause, dialog hiding/restoration, and cleanup.
  Automatic graphics startup failure falls back to Text with a notice; forced
  output failures remain errors. All four music video graphs produce both
  audio and video in real mpv.

## Final validation

- `go build ./...` and a Windows/amd64 cross-build pass.
- `PLEXCTL_TEST_MPV=/usr/bin/mpv go test -race -shuffle=on -count=1 -tags=integration ./...`
  passes. Local HTTP/IPC fixtures use temporary caches and synthetic media.
- Both normal and integration-tagged `golangci-lint run` pass with zero issues;
  `gopls check` reports no diagnostics in the changed core files.
- `go mod verify`, `go mod tidy -diff`, and `git diff --check` pass.
- Go 1.27-compatible `govulncheck` reports no reachable vulnerabilities. It also
  reports 13 module-level advisories in code that this application does not call.
- The documentation site builds all 49 pages. ESLint passes with one existing
  `img` optimization warning in `docs/app/layout.jsx`; generated `out/` files
  are now excluded from linting.

## Local compatibility finding and manual acceptance

The development mpv on this machine's PATH identifies itself as
`v0.41.0-1150-g88eb9f2e7`. It aborts inside `vo_sixel.c: slice_dither` with a
palette-size assertion, reproducible by running mpv directly on synthetic audio
without plexctl. Changing palette options did not resolve it. The independent
system `/usr/bin/mpv` 0.37.0 passes the same Sixel overlay test. Neither installed
player was modified. `PLEXCTL_TEST_MPV` selects the player for the overlay
integration test; other tests continue to use mpv on PATH.

Physical appearance and performance still need interactive checks on Kitty,
Ghostty, a Sixel terminal, and tmux: browse and clip posters, open/close dialogs,
switch poster renderers, resize paused overlay video, and return from suspended
playback. Headless protocol and real-player tests cannot establish visual quality
on those terminal emulators. Live Plex library and watch-history acceptance was
not performed; local fixtures validate pagination, server credentials, and
timeline requests without accessing the user's media.

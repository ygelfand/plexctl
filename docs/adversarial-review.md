# Adversarial review — 2026-10-17

Reviewed the Go CLI/TUI, Plex authentication and HTTP clients, caches, background
requests, playback processes, mpv setup, and the documentation application's
dependency tree. Changes remain in the working tree.

## Fixed findings

| Priority | Finding and trigger | Result |
| --- | --- | --- |
| P1 | Verbose HTTP dumps included Plex tokens, Home PINs, response credentials, and URL secrets. Device JSON/YAML output also included resource tokens. | HTTP diagnostics retain only method, safe URL, status, and timing. Device output strips credentials without modifying the original objects. |
| P1 | Config writes used permissive permissions and could truncate credentials on failure. JSON persistence omitted resource tokens and did not consistently preserve field names. | Saves use a private temporary file and atomic replacement. YAML, JSON, and TOML round-trip credentials and library settings. |
| P1 | A server could redirect authenticated HTTP requests to another origin. A malformed media/subtitle/image path could change the URL authority when concatenated with the server URL. | Authenticated HTTP clients enforce the original origin. Parsed media paths preserve the configured destination and replace metadata-supplied token parameters. Artwork credentials travel in headers. |
| P1 | A saved shared server could receive a broad account token after the account changed; connection probes sent credentials before verifying which server occupied a private address. | Missing scoped shared-server credentials fail closed. Public identity probing precedes authenticated access checks. |
| P1 | History reused the first authenticated client after a profile switch. Background indexing could fetch and save data under different profiles. Account-only commands used server resource credentials. | Stores, indexes, loaders, and view requests capture their originating identity. Account and server clients use separate credentials; caches include endpoint and credential scope. |
| P1 | Sessions, Tasks, and Libraries all returned indistinguishable row slices; a late reply could populate a different tab and supply the wrong identifier to an action. | Results identify their originating view and refresh generation. The controller delivers results to that view even when it is inactive. Replaced views cannot consume old results. |
| P1 | Tiny compressed artwork with huge dimensions, oversized downloads, or extreme watched offsets could cause excessive allocation. | Artwork downloads and decoded dimensions are bounded. Poster progress bars clamp metadata offsets before calculating their size. |
| P2 | Terminal mpv inherited a two-minute request deadline, terminating long playback. | Playback retains cancellation for stop/replacement; metadata and startup have independent deadlines that do not limit running playback. |
| P2 | An exited mpv/tmux wrapper could leave a child holding stdout, hanging a supposedly timed probe. | Probe subprocesses use bounded pipe waits. Subprocess regressions cover inherited stdout. |
| P2 | History workers mutated the UI cursor, repeated pending pages, skipped days on errors, and overlapped inclusive day boundaries. User-picker, reload, and delayed navigation results could outlive their originating profile. | Workers return immutable page results. Requests carry generations, failed pages retain their cursor, day windows do not overlap, and stale switch/reload/navigation results are ignored. |
| P2 | Missing metadata containers, missing season keys, and sparse butler task fields could panic. Failed index child requests could silently replace a valid index with partial data. | Responses are validated, unavailable task identifiers are excluded, and failed reindexing preserves the previous index. |
| P2 | Cache initialization raced and ignored subsequent cache directories. Binary image data was inserted as invalid raw JSON. Cache write failures could discard otherwise successful network results. | Managers are synchronized and keyed by directory. Binary values round-trip through JSON encoding. Optional cache failures preserve fetched results. |
| P2 | Pagination accepted invalid/overflowing offsets, repeated non-advancing pages, and premature empty pages. | Invalid offsets and incomplete/repeated listings return errors; cancellation stops the walker. |
| P2 | Malformed or explicitly missing config files were silently ignored. A missing configured server entry was treated as usable. | Configuration errors are surfaced and server availability requires an actual endpoint. |
| P2 | A queued poster selection indexed a list after a refresh changed it. | Selection commands capture metadata before dispatch; title truncation preserves Unicode display widths. |

Regression coverage is in the adjacent `*_test.go` files, including local HTTP
fixtures, stale-message delivery, profile changes, binary image caching,
malformed metadata, virtual-clock playback, and subprocess pipe inheritance.

## Go dependency findings

The initial `govulncheck` scan found no vulnerable imported packages or reachable
symbols, but reported 13 advisories in unused packages of `golang.org/x/image`
and `golang.org/x/net`. Both modules were updated to the current stable versions
verified through the Go module proxy and upstream commit histories:

| Module | Before | After |
| --- | --- | --- |
| `golang.org/x/image` | v0.38.0 | v0.47.0 |
| `golang.org/x/net` | v0.59.0 | v0.61.0 |
| `golang.org/x/sys` | v0.48.0 | v0.49.0 |
| `golang.org/x/term` | v0.46.0 | v0.47.0 |
| `golang.org/x/crypto` | v0.57.0 | v0.58.0 |

The latter three updates follow the upstream module requirements. The project's
Go requirement remains 1.27.2. The directly used `image/draw` and `net/html`
packages are unchanged between the old and new releases. The final scan reports
no vulnerabilities, including at module level.

## Documentation dependencies

The documentation app exports static files to GitHub Pages. Compatible fixes
were applied without changing the Nextra framework version:

| Package | Before | After |
| --- | --- | --- |
| Next and its two ESLint companions | 16.4.0 | 16.4.2 |
| `@xmldom/xmldom` | 0.9.10 | 0.9.12 |
| `source-map-js` | 1.2.1 | 1.2.2 |
| `ajv` | 6.12.6 | 6.15.0 |

The xmldom override applies only to `speech-rule-engine`, whose latest stable
release pins the vulnerable version. Other transitive updates satisfy existing
parent ranges. Next and its ESLint companions stay aligned. The
[16.4.1 security fixes](https://github.com/vercel/next.js/releases/tag/v16.4.1)
are included in [16.4.2](https://github.com/vercel/next.js/releases/tag/v16.4.2);
the affected server and image endpoints are not deployed by this static export.

The final npm audits still exit with status 1:

| Audit | Before | After |
| --- | --- | --- |
| Full dependency tree | 17: 8 high, 2 moderate, 7 low | 13: 6 high, 7 low |
| Production dependencies | 14: 6 high, 1 moderate, 7 low | 11: 4 high, 7 low |

Those are propagated package counts for two remaining upstream advisories:

- [braces: GHSA-vfj7-8cjw-p6xm](https://github.com/advisories/GHSA-vfj7-8cjw-p6xm).
  Attacker-controlled nested glob patterns can exhaust the stack. No patched
  release exists. Here, Nextra and Next ESLint reach it through fast-glob and
  micromatch using repository paths and build/lint configuration. Static-site
  visitors cannot supply these patterns.
- [KaTeX: GHSA-238p-pmpm-9mq7](https://github.com/advisories/GHSA-238p-pmpm-9mq7).
  The bypass requires existing prototype pollution or attacker-controlled
  option prototypes, together with attacker-controlled math expressions.
  KaTeX does not introduce the pollution. Version 0.18.2 fixes it, but lies
  outside the current parents' 0.16 ranges; no compatible Nextra update exists.
  This app renders repository-owned MDX, with no identified input path for
  untrusted expressions/options or prototype pollution.

Neither remaining advisory has a demonstrated exploit path in this application.
They remain recorded rather than being hidden by audit exceptions or a forced
framework downgrade. Reassess them if the site accepts untrusted content or
changes to a server deployment.

## Validation

Passed:

- `go mod verify` and `go mod tidy -diff`.
- `go build ./...`.
- `go test -race -shuffle=on -count=1 -tags=integration ./...`.
- Both `golangci-lint run` and `golangci-lint run --build-tags integration`.
- `govulncheck ./...`, using v1.8.0 built with Go 1.27.2.
- Windows amd64 and macOS arm64 builds with CGO disabled.
- Focused race and lint checks for the final poster bounds fix.
- Documentation: clean `npm ci`, valid `npm ls`, and `npm run build` producing
  49 static pages, using npm 11.21.1.
- Documentation `npm run lint`: zero errors, one existing `no-img-element`
  warning in `app/layout.jsx:30`.
- `git diff --check`.

Security scanner diagnostics were traced through their call sites. Remaining
heuristic reports concern graphics IDs that are not secrets, non-cryptographic
cache hashes, bounded numeric conversions, locally generated paths, structured
subprocess arguments, and intentionally readable public build outputs. They do
not represent demonstrated exploits in these uses; no blanket suppressions were
added.

Runtime integration tests ran on Linux. Windows and macOS were compile checks.
Authentication and HTTP regressions use local server fixtures; no live
authenticated Plex account was used for validation.

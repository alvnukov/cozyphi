# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- Default theme is now `opencode` (dark), ported from the opencode TUI
  palette: warm orange primary, blue secondary, near-black grays. A matching
  `opencode-light` variant joins the `/theme` picker, which now leads with
  both; `Terminal` (ANSI-follow) remains available.
- Transcript answers render with opencode-style typography: word-aware
  wrapping (words never split mid-grapheme), hanging-indent lists, blockquote
  rules on every wrapped line, and fenced code in a rounded box with the
  language in the top border. The muted theme color drops its extra dim so
  markers and metadata stay readable, and user prompts get an accent left bar.

### Added

- Slash command set grows to opencode parity: `/new` (fresh session, alias of
  `/clear`), `/compact` (summarize the history now — same pipeline as
  auto-compaction, feedback lands in the transcript), `/export [path]` (write
  the transcript as markdown, default `phi-<session>.md` in the cwd),
  `/theme <name>`, and `/model <name>` (registered from the configured model
  list). Commands parse arguments and tolerate extra whitespace; slash-looking
  prose (`hello /clear`, `/etc/hosts …`) still goes to the model untouched.
- The `/` picker now also completes command arguments: typing after
  `/theme ` or `/model ` offers matching values in the same menu, and accept
  replaces just the argument.
- Tab toggles the agent posture between `⏵⏵ build` and `⏵⏵ plan`
  (label on the composer's top-left border, opencode-style). Plan mode swaps
  the system prompt to a read-only planning brief, drops `write`/`edit` from
  the tool list, and overlays a readonly permission policy — mutating bash is
  folded to the allowlist; reads and checks (`git diff`, `go test`) keep
  running. The `@` picker now also completes sub-agent roles (`@explore`,
  `@review`, `@worker`): a prompt starting `@role task` is delegated through
  `agent_spawn` and the sub-agent summary is relayed back.
- Completed assistant turns end with a muted opencode-style metadata row —
  `• model[context] • 1m 4s` (model, context tokens, wall time). It rides the
  final text of each round, never enters copy/selection, and replayed history
  (no stored timings) shows just the model.
- Status sidebar (`Ctrl+O`): right-hand panel with the context-window fill
  bar, per-turn token usage for the last five turns, and configured MCP
  servers. Hidden by default and suppressed under 110 terminal columns so the
  chat keeps at least 80.
- `phi run --yolo`: skip all permission checks for one headless run (benchmarks / CI).
- `PHI_PPROF=host:port` serves `/debug/pprof` from the TUI for hang diagnosis.
- `phi -c` / `phi --resume <id>`: start the TUI directly on the newest session for
  the directory, or on a session by id / unique prefix (same flags after `phi tui`).
  Session resolution happens before the UI starts — typos exit 3 with a one-line
  error — and the resumed history is already in the transcript on the first frame.
- Hooks: session lifecycle events now include `usage` — token counts of the latest completed assistant turn.
- Hooks: `post_turn` event fires after each completed assistant stream with per-round `usage` (for audit metrics such as cache hit ratio).
- Agent: new `context` tool for the model — reports quantitative context
  usage (tokens with source, serialized KB, window, compact threshold,
  recommendation; never conversation content) and adds an explicit `compact`
  action. Requested compaction applies at the tool-round boundary, keeps
  recent messages verbatim, and never deletes the on-disk transcript.

### Fixed

- Config: `config.yaml`, its `.bak`, and session transcripts are written 0600
  (owner-only) — API keys and transcripts no longer sit world-readable — and
  `GET /api/config` masks stored API keys instead of returning them.
- Config editor: renaming a model while its api_key is masked now fails the
  save with an actionable error naming the model (keep the name or re-enter
  the key); previously a non-default model silently lost its stored key.
- Sessions: resuming a legacy 0644 transcript tightens the file to 0600 on the
  next write, instead of appending to a world-readable file (0600 used to apply
  only at file creation).
- Resume: an ambiguous id / id-prefix error now lists the matching ids (capped
  at five, "+ N more" after) instead of a bare match count, so the prefix to
  retype is right there.
- TUI: the footer shows the current session's short id from the first frame on
  (idle: alone; busy: after the activity label), so a resumed session is
  identifiable without waiting for a toast.
- TUI: permission prompts are on by default — `dangerously_allow_all` (or the
  `--yolo` flag for `phi run`) is now required to skip them; previously the
  gate defaulted to bypass even when the config omitted the key.
- TUI: quitting with Ctrl+C now runs `session_shutdown` hooks and closes the
  job manager and MCP servers (previously the close call sat on a
  never-reachable path, so quitting leaked hooks, sub-agents and MCP servers).
- TUI: Ctrl+C quit hung the process forever — the tty reader never woke on
  `Loop.Stop` because read deadlines cannot reach `/dev/tty` on darwin. Reads
  in raw mode are now bounded by `VMIN=0/VTIME=1` (100 ms), so quit completes
  promptly.
- Agent: `SetModel`/`SetJobs` rebuild the tool list from the engine's
  configured tool set instead of `DefaultTools` — read-only engines
  (sub-agents) no longer silently gain `write`/`edit` after a setter call.

### Changed

- TUI renders on demand instead of a constant 60 fps ticker: idle sessions
  write zero bytes to the terminal and use no CPU. Footer spinner and toast
  expiry drive their own frames via `DrawContext.Wake`.
- Vendored `xui` (via `go.mod` `replace`): the renderer keeps a cursor diff
  cache, skips empty frames, and hides/shows the cursor only on frames that
  paint — fixing the idle cursor jitter.
- The welcome screen is now a static centered CozyPhi wordmark with tagline,
  version, and shortcut hints; the animated splash sphere and its 30 fps
  redraw loop are gone — an idle welcome screen paints nothing.
- The fork numbers its own releases: version starts at v0.1.0 (upstream phi
  was at v0.16.0). The update check and `phi update` now target
  `alvnukov/CozyPhi` releases instead of upstream `pulseaiclub/phi`.

### Deprecated

### Removed

### Security

- Agent: `agent_spawn` workdir is validated at spawn time against the parent
  session workspace: a workdir resolving outside it fails the tool call with
  an actionable error instead of silently becoming the child's write
  boundary. Relative workdirs resolve against the parent cwd, and the child
  runner re-asserts the boundary before assembling its permission gate.

<!-- Released section -->
<!-- Don't change this section unless doing release -->

## [0.16.0] - 2026-08-22

### Added

- Hooks: `command` UI intents — `status` (footer), `list` (palette page).
- Hooks: session lifecycle events `session_start`, `session_shutdown`, `session_before_switch`.

### Changed

### Deprecated

### Removed

### Fixed

### Security

## [0.15.0] - 2026-08-20

### Added

### Changed

### Deprecated

### Removed

- `agent_list` `status` filter parameter (always returns the full list; each row still includes `status`).

### Fixed

### Security

## [0.14.0] - 2026-08-20

### Added

- Hook event `command`: `plugin.json` entries register TUI slash commands (`/name` runs `run`). stdout may `submit` a user message or `toast`.

### Changed

- `write` creates or overwrites files (no longer create-only). Use `edit` for surgical changes.
- File tools resolve relative paths against the session cwd and print cwd-relative results (`find`/`ls`/`grep`/`read`/`write`/`edit`). Absolute paths are used internally (including rg/fd) and returned only when the file is outside cwd.
- `find` (formerly `glob`) uses `fd` from `~/.phi/bin` (same as `rg`): respects `.gitignore`, early-stops at limit, optional `limit` arg.
- Renamed directory listing tool `list` → `ls`.

### Deprecated

### Removed

- Built-in `fetch` tool (and `permissions.fetch` config). Use MCP if you still need URL fetching.
- `agent_log` tool (parent agents only get `agent_wait` summaries; job logs remain on disk under `~/.phi/jobs/`).

### Fixed

- `phi update` on Windows: stage the download next to the installed binary (same volume) and fall back to copy when rename still cannot cross drives.
- Assistant fenced code blocks drop the box/`-----` chrome; a muted language caption sits above the highlighted code so mouse selection stays copy-clean.

### Security

## [0.13.0] - 2026-08-18

### Added
- TUI hot-reloads the git branch in the path label: switching branches outside the app (another terminal, an editor) refreshes the label automatically.

### Changed
- TUI activity: tool rows keep a 1-cell braille spinner; the footer uses an
  Knight-Rider scan bar so the two don't share the same glyph.
- Tool routing: bash is no longer described as an inspection tool; grep/glob no
  longer nudge `agent_spawn`; `edit.hash` is the 4 hex chars after `#` in
  `@file path#TAG` (leading `#` / full header copy-paste is accepted).
- **Breaking:** hooks are declared in `plugin.json` (one file, many hooks) instead of
  per-directory `hook.json`. Load `~/.phi/hooks/plugin.json` and
  `~/.phi/hooks/<plugin>/plugin.json` (same under the project `.phi/hooks/`).

### Deprecated

### Removed
- Per-hook `hook.json` directories. Use `plugin.json` instead.

### Fixed

### Security

## [0.12.0] - 2026-08-17

### Added

- Changelog gate: PRs must update `CHANGELOG.md` (with skip labels / `[chore]`), released sections are protected, and GitHub Release notes are taken from this file.

### Changed

- Hashline `edit` now requires a whole-file `@file path#TAG` (`hash` field) from `read`/`grep`; after a successful edit, re-read before another `edit` on that path. Per-line hashes are 3 letters (a-z) and no longer use digits.

### Removed

- Remove the redundant `agent_task` tool; compose `agent_spawn` + `agent_wait` instead.

## [0.11.0] - 2026-08-16

Baseline release when this changelog became the source of truth for user-visible changes.
Earlier releases are available from GitHub tags only.

<!-- Released section ended -->

[Unreleased]: https://github.com/pulseaiclub/phi/compare/v0.16.0...HEAD
[0.16.0]: https://github.com/pulseaiclub/phi/releases/tag/v0.16.0
[0.15.0]: https://github.com/pulseaiclub/phi/releases/tag/v0.15.0
[0.14.0]: https://github.com/pulseaiclub/phi/releases/tag/v0.14.0
[0.13.0]: https://github.com/pulseaiclub/phi/releases/tag/v0.13.0
[0.12.0]: https://github.com/pulseaiclub/phi/releases/tag/v0.12.0
[0.11.0]: https://github.com/pulseaiclub/phi/releases/tag/v0.11.0

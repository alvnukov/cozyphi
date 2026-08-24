# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `/connect` now provides a validated, restart-safe provider catalog with
  pinned subscription integrations: OpenAI Codex through the ChatGPT device
  sign-in flow and Z.AI Coding Plan through its dedicated API key. Both use
  the OpenAI Responses protocol, preserve stateless reasoning/tool state, and
  keep credential refresh off the UI thread.
- Prompt history for the composer: Up from the first line recalls the previous
  submission, Down walks back toward the draft (bash-like; multiline drafts
  keep caret movement). History persists in `~/.phi/prompt-history.jsonl`,
  capped at 50 entries, and survives restarts.
- Hover pointer shapes via OSC 22 (kitty 0.31+, ghostty, foot; other terminals
  keep their default pointer): a hand over clickable spots — expandable block
  title rows, tappable footer rows — a text beam over selectable and editable
  text, and a horizontal-resize arrow over the sidebar border.

### Fixed

- Sidebar panel text no longer hugs the frame: a one-cell gutter now separates
  every block from the left and right borders, the first row sits below the
  labeled top edge, and the plan scroll thumb lives in the gutter instead of
  overwriting the border.
- Reading history while the model streams no longer yanks the view: content
  growth below the viewport extends the scroll extent instead of shoving the
  visible text downward. Follow mode (at the bottom) still tracks the tail.
- Drag-selection now auto-scrolls at the transcript edges, so a selection can
  span several screens: a slow crawl in the rows just inside an edge, faster
  on the edge row, faster still (scaling with depth) once the pointer leaves
  the list into the composer zone. Scrolling continues while the button is
  held, the selection endpoint rides along, and both edges stop at the
  content bounds.
- Busy state has one source of truth: the controller reports whether a run or
  queued prompt is in flight, and every input gate (composer submit, Esc,
  `/clear`, `/compact`, hook commands) asks the same `CanSubmit` predicate.
  The footer activity no longer reconciles from the transcript snapshot, so a
  desynced activity enum can neither block submits ("cannot submit" while
  idle) nor stick the footer on a stale label after a run ends.
- The composer panel background no longer breaks under the placeholder, typed
  text, and the meta row — every style painted inside the frame now carries
  the element panel color.

### Changed
- The Ctrl+O right sidebar now keeps model/mode/activity, context and live MCP
  connection states fixed above an independently scrollable session plan. The
  primary model uses one `plan` tool with `get` and revision-checked `update`
  actions for the exact durable snapshot, with bounded notes,
  completion evidence, and a `blocked` status. Resume and compaction retain the
  plan while inference receives only a constant-size status hint; full step
  text is fetched on demand. Drag the panel's left border to resize it; the
  width is restored on the next launch.
- Transcript message layout follows opencode's session view: the list insets
  entries two columns per side; user prompts render as panels (blue ┃ rule,
  panel background, breathing room above and below the text); assistant
  answers, thinking, tool, bash and sub-agent blocks share a three-column left
  rail; the end-of-turn footer reads `▣ model[context] · duration` with the
  marker blue, the model bright and the rest muted; compaction shows a
  centered ` Compaction ` rule instead of an italic word. Legacy themes keep
  their previous chrome.
- The compaction rule carries the before/after report and, when a summary was
  generated, expands on Enter or a click on the rule — ▶/▼ affordance — to
  show the dim summarize body, so "what did compaction fold away" is one
  keypress away instead of invisible.
- Transcript spacing breathes again: entries separate by two blank rows, the
  first message drops one row below the top edge, and a blank gap row sits
  between the transcript and the composer frame (collapsing on screens too
  short for both floors) — opencode's paddingBottom rhythm instead of a
  glued column.
- Default theme is now `opencode` (dark), ported from the opencode TUI
  palette: warm orange primary, blue secondary, near-black grays. A matching
  `opencode-light` variant joins the `/theme` picker, which now leads with
  both; `Terminal` (ANSI-follow) remains available.
- Transcript answers render with opencode-style typography: word-aware
  wrapping (words never split mid-grapheme), hanging-indent lists, blockquote
  rules on every wrapped line, and fenced code in a rounded box with the
  language in the top border. The muted theme color drops its extra dim so
  markers and metadata stay readable, and user prompts get an accent left bar.
- The composer input now renders in opencode's prompt frame: a left ┃ bar in
  the posture color wraps a backgroundElement panel, the `⏵⏵ build · model`
  meta row sits inside the frame bottom, a ╹▀ tail fades the frame into the
  terminal, and the row below shows the cwd muted on the left with usage stats
  right-aligned (a `tab mode · ^k commands` keymap fallback when no usage is
  reported).
- An empty composer shows opencode's muted placeholder — `Ask anything...`,
  swapped for `Run a command...` while a `!` shell prefix is active — and the
  composer's minimum height now lives in `ChatInput.MinHeight`, consumed by
  the pane and the editor layout instead of being re-derived at each call
  site.
- Submitting while a run is still streaming now queues the prompt (shown
  immediately in the transcript) instead of dropping it; it runs as soon as
  the current turn finishes.

### Fixed
- Interrupted tool rounds no longer leave provider-invalid session history:
  cancellation records a result for every advertised tool call, resume durably
  closes a broken tail, and legacy orphaned/partial results are repaired in the
  provider context before Anthropic or OpenAI-compatible requests are sent.
- Thinking blocks render collapsed by default — streaming included: the
  header spinner is the activity signal, and the reasoning body appears only
  after Enter/space/click expands it (the choice sticks for the session). A
  finished block no longer lies "Thinking": the header reads
  `Thought for 4s` (opencode-style span, measured from the first reasoning
  delta to the first answer delta), plain `Thought` when the span is unknown
  or under a second, and `Thinking (interrupted)` stays for cancelled rounds.
- The hard-coded 4096 output-token cap is gone: a reply's token budget is now
  `models[].max_output_tokens` in config.yaml (editable in `phi config`,
  empty = provider default; Anthropic-shaped endpoints get a provider-safe
  8192 fallback because that API requires the field). Provider finish reasons
  are parsed end-to-end, so a reply cut off by the limit is never silent —
  the turn footer reads `hit max tokens`, and a round truncated before any
  text shows an explicit warning row naming the setting.
- Choosing "allow all for every session" in a permission dialog no longer
  corrupts config.yaml when the permissions section was saved inline
  (`permissions: {}`, what the config editor writes for an untouched
  section): the setter rewrites it as a block, refuses non-empty inline
  mappings instead of mangling them, and no longer doubles the file's final
  newline.
- Resumed sessions restore provider usage immediately. Manual compaction now
  summarizes older turns below the automatic threshold, preserves the current
  turn, reports before/after context metrics, and refreshes context state
  without waiting for another model response.
- Stopping a running turn (including Esc during a provider error, tool update,
  or compaction) now terminates the event iterator immediately. No layer calls
  a closed range callback, and pending tools/model rounds do not continue after
  the consumer stops.
- Growing assistant Markdown now commits completed top-level blocks, reparses
  only the mutable tail, and repaints only changed visual rows. Cached surfaces
  use copy-on-write for selection highlights, so long answers remain responsive
  without leaking highlight state into later frames.
- Streaming updates now mutate an owned session reducer and re-project only the
  unchanged-shape tail rows. Long transcript history no longer adds work to
  each token; structural changes and replay fail closed to a full sync.
- Keyboard redraws reuse the parsed layout of unchanged assistant Markdown,
  so typing and navigation no longer reparse a long streaming answer on the
  UI goroutine. Text, state, metadata, theme, width, and terminal width mode
  all invalidate the cache explicitly.
- Prompt cancellation no longer marks the controller idle before the engine
  loop exits, preventing a fast post-Esc submit from running two turns against
  one session. Accepted queued prompts survive cancellation, selected skills
  are snapshotted, session/model mutations reject active runs, and prompts are
  never started concurrently with local shell commands without feedback.
- Background stream redraws are paced at 20 fps while keyboard redraws remain
  immediate. Long Markdown replies no longer consume nearly a full CPU core at
  60 terminal frames per second and starve input events.
- Transcript colors follow the real opencode palette. The theme port covered the
  12 chrome roles but markdown and code highlighting improvised (H1 green, H2
  blue, H3 orange, inline code orange, keywords blue-bold, no-language code
  boxes orange). `Theme` now carries `Markdown` and `Syntax` role groups ported
  verbatim from opencode's `opencode.json`: purple bold headings (H1
  underlined), orange strong, yellow emphasis and quotes, green inline code,
  cyan link labels, peach/cyan list markers, and opencode's syntax palette for
  code (purple keywords, red variables, cyan operators). Paths in prose keep
  the text color and only gain an underline. Dark/Darcula/Pink/Terminal keep
  their previous look through the same roles.

- Border labels (model name, build/plan posture, token stats, cwd) and the
  footer status line now truncate with an ellipsis instead of a hard cut —
  a long model name no longer renders as `deepseek-v4-pro-`, and a long
  status no longer slides under the update hint on narrow terminals.
- Esc works everywhere again. The input parser held a lone Esc byte forever
  as a potential sequence prefix, so no Esc handler ever fired — the
  permission overlay ignored its own "Esc cancel" hint (and the next keypress
  was swallowed as an Alt shortcut). The read loop now delivers a held lone
  Esc as a key press once input stays quiet for 50 ms.

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
  bar, latest token usage, configured MCP servers, and the durable agent plan.
  It is visible by default, remembers its last visibility and width globally,
  shows a `Ctrl+O hide` hint, and yields to the chat on narrow terminals.
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

- Sidebar token usage now replaces the current row instead of adding another
  row after every model/tool round.

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

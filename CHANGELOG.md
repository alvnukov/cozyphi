# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
- `plan` is now steps-only: the model sends `{"steps":[...]}` and the harness
  atomically replaces the current plan under one lock, owning the revision.
  `plan action=get` is removed — the authoritative <current-plan> snapshot is
  injected into every inference as a transient message instead of a tool result,
  so the model always sees the current plan without an extra round. Legacy
  `{action:"update", expected_revision:N}` calls are tolerated, but `get` is
  rejected.
- The agent can now watch things in the background instead of polling for them.
  A new `watch` tool starts one of three shapes: a **stream** (a command whose
  matching output lines are events, or one event on exit with the code and
  tail), a **poll** (a command re-run on an interval, firing only when its
  output changes), or a **timer** (a plain reminder, optionally one-shot).
  Events reach the user as a transcript row immediately and the model as a
  `<system-reminder>` — injected into a running turn at its next tool round, or
  starting a turn when the session is idle. `list`, `log` and `stop` round out
  the tool.
- The agent is actually told about watches: the system prompt routes work that
  takes minutes — or that would otherwise be re-run to check on — to `watch`
  rather than to a `bash` call waited out or repeated, and only when the engine
  carries a watch manager. The tool's own description covers the ways a watch
  fails silently: a filter matching only the success line, a pipe stage that
  buffers instead of flushing, a poll whose output carries a clock so every
  tick reads as a change. The caps it quotes are rendered from the constants
  that enforce them.
- Watches are bounded on every side: the permission gate judges one by the bash
  deny list and default and never by the bash allowlist (an entry that clears a
  command to run once does not clear it to run forever, so `tail -f` still
  asks); a watch emitting more than 20 events a minute stops itself; 8 may run
  at once; and watches may start at most 5 turns in a row without user input —
  past that events wait and ride along with the next prompt. `Esc` calls off a
  pending wake. Watches live as long as the process, are not persisted, and are
  given to neither sub-agents nor headless `cozyphi run`.
- Cozyphi and Claude Code now share per-repository auto memory at
  `~/.claude/projects/<encoded-git-root>/memory/`: every worktree and
  subdirectory uses the same Markdown topic files (`user`, `feedback`,
  `project`, `reference`) and Claude-compatible `MEMORY.md` catalog. Legacy
  `~/.cozyphi/memory/` data is neither read nor imported. Kind decides how a
  fact reaches the model: `user` and `feedback` ride in the system prompt in full, `project` and
  `reference` are named there and arrive whole, as a `<system-reminder>`, on
  the turn they match. Matching weighs a word by how rare it is in the
  directory, folds it to a prefix so Russian inflection still matches, counts a
  path or identifier triple, and reads the turn — the prompts around it and the
  files its tools touched — not just the last message. Every tier is capped, so
  memory cannot grow the prompt: a 200-fact directory costs ~1.5k tokens, and
  `cozyphi memory` prints the number.
- Memory forgets and compacts without losing anything. What the prompt has no
  room for stays findable by retrieval and by the new `memory` tool (`list`,
  optionally ranked by a query; `read` by name; `overlaps`; `forget`), which
  never writes a fact — creating and changing one stays with `write`, through
  the permission gate. What gets used keeps its place, counted in
  `~/.cozyphi/usage.json` beside the picker history; `forget` moves a file into
  `forgotten/` instead of deleting it; `pin: true` is never demoted or called
  stale; exact duplicates are archived automatically; and when the directory is
  crowded, idle or repetitive the prompt says so and names what to merge.
  Sub-agents carry no memory, and so no tool. New: `cozyphi memory
  [list|path|show <name>|forget <name>|forgotten]`.
- The per-turn tool-round budget is raised to 128. The sidebar's status area is
  now a Status/Settings tab window, visually separated from the plan by a blank
  row and a pane divider; the plan and its `approved`/`auto` checkboxes keep
  their place on both tabs. The tab window no longer duplicates model, mode or
  run state already shown in the main UI. Settings adds a `stop@128` checkbox
  (persisted to `~/.cozyphi/ui.json`) that toggles the hard stop. The plan pane
  gains a `clear` action beside `auto` that drops the plan and resets its
  revision counter.
- Slash commands, model picker, skills, and command-palette leaf actions now
  order by local usage history (frequency + recency), while new items and equal
  ratings keep their built-in order. History lives in `~/.cozyphi/usage.json`
  (owner-only, 0600) and is written only after a successful use; nothing leaves
  the machine.
- Approving an active plan now immediately hands control back to the agent;
  completed plans remain idle instead of restarting.
- Added a project README with installation, quick-start, and TUI screenshot.
- Completing an auto-approved plan no longer loops approval updates, freezes the
  TUI, or rapidly grows the session transcript.
- Plan reads no longer fail when a tool client fills update-only fields with zero
  values: `get` remains side-effect-free, `update` validation stays strict, and
  the prompt now shows the exact minimal `get` payload.
- The model can now ask interactive questions: a new `question` tool offers
  selectable options (with multi-select and a type-your-own row) that you answer
  with ↑↓/enter instead of typing prose.
- The plan sidebar gained an `auto` checkbox that approves an incoming plan
  without a manual click, and the `approved` checkbox drops once every plan
  step is closed.
- TUI polish: the transcript log is pulled toward the left edge (tighter padding
  and message indent), process lines use the `❋` marker and messages use `»`,
  and the spinner animates a letter-gradient wave across `Thinking`.
- The spinner now advances on wall-clock time instead of per draw frame, so
  mouse movement that triggers redraws no longer speeds up the animation.
- The status sidebar now shows token usage as labeled rows (`in` / `out` /
  `cache` / `total`) instead of a run of compact `↑##k C## Σ##` symbols, so
  the count breakdown reads at a glance.

- Paste an image from the system clipboard into the composer: it is attached to
  the prompt (shown in the hints row), and sent to the model as an inline image
  content part across Anthropic, OpenAI chat, and OpenAI-responses protocols.
  Alt+X removes the attached image before submitting.

## [0.17.0] - 2026-08-26

- Fix Windows release builds: the SIGCONT repaint-on-resume path is now
  Unix-only with a no-op Windows stub, so cross-compiling no longer fails on
  `undefined: syscall.SIGCONT`.

- Advertise hierarchical document symbol support so gopls returns precise
  selection ranges: `lsp` definition, hover, references, and call hierarchy
  now resolve symbol-name targets against real gopls instead of returning
  empty results or "no identifier found".

- The `lsp` tool now accepts the plan gate's `plan_step` argument instead of
  rejecting it as an unknown field, and call hierarchy uses the protocol-3.17
  method names gopls registers (`textDocument/prepareCallHierarchy`,
  `callHierarchy/incomingCalls`, `callHierarchy/outgoingCalls`) — real-gopls
  call hierarchy now resolves instead of returning "method not found".

- The status sidebar now lists connected LSP servers alongside MCP, with the
  same state markers, and section headers are rendered in normal (non-muted)
  uppercase instead of lowercase gray.

- Bound the `lsp` lifecycle: 15s handshake and query deadlines (30s for
  workspace symbols), a per-root circuit breaker (3 starts per 60s with
  `retry_after_seconds`), an exact close order (cancel pending → didClose →
  shutdown → exit → kill), and fuzz coverage of framing, URIs, location
  shapes, markup, and diagnostics. Fuzzing found and fixed a crash on an
  empty hover payload. The system prompt now routes Go navigation questions
  to `lsp` when the tool is enabled.
- Add owner-controlled LSP configuration: `~/.cozyphi/lsp.json` sets the
  gopls command, env additions, initialization options, and settings, loaded
  as a secure owner-owned regular file with fail-closed validation — symlinked,
  foreign-owned, group- or world-writable, malformed, or unknown-key configs
  are rejected. Bare executable names resolve through `~/.cozyphi/bin` and
  then PATH, never the working directory. The `lsp` tool gains a `languages`
  operation reporting configured, installed, running, active roots, supported
  operations, and an install hint without ever starting a process.
- Add document synchronization and diagnostics to the `lsp` tool: didOpen
  and didChange with negotiated full or incremental UTF-16 sync, bounded
  document tracking with didClose on eviction, and a diagnostics operation
  that merges proven-current push and pull reports into fresh, cached,
  unconfirmed, or pending results — never a false empty-success claim.
- Add harness-managed `lsp` tool with exact-position gopls definition,
  bounded JSON-RPC framing, physical path containment, and graceful shutdown.
- Add navigation operations to the `lsp` tool: symbol-targeted definition,
  references with include_declaration, hover across every contents shape,
  document/workspace symbols, and incoming/outgoing call hierarchy — bounded,
  deduplicated, capability-gated, never exposing raw protocol payloads.
- The job manager now reaps every live job even when Close is called with an
  already-cancelled context (as t.Context() does before test cleanups), so a
  finishing runner can no longer write into directories being removed.
- Rune-bound hotkeys (vim navigation, Ctrl+K palette, Ctrl+A approve, copy,
  modal/picker chords) now match regardless of keyboard layout: a Russian
  (ЙЦУКЕН) layout's letters map back to the same physical US-QWERTY key.
- Interface copy is now uniformly English (plan approved/stopped toast and
  sidebar checkbox label).
- A message submitted mid-run now stays in place in the transcript until
  the model receives it, and the model's answer renders below it: the bus no
  longer lets one tool round's first delta swallow the previous round's
  terminal event, which used to rewrite the model's answer above the queued
  message.
- A message queued while the model is tool-looping now reaches the model at
  the next tool-round boundary — inside the same turn — instead of waiting
  for the whole agentic turn to end; the "(queued)" hint clears the moment
  the model sees the message.
- A message sent while the model is still working is now shown with a
  "(queued)" hint in the transcript until the running turn finishes, so it is
  clear the message is waiting rather than already sent.
- Sending a message while the model is still working now answers it when the
  current turn finishes (the message queues behind the running turn, as in
  Claude Code/opencode). The in-flight turn used to stay stuck in a streaming
  state after the queued message was inserted below it, so the follow-up never
  reached the model.
- Context browser (/context): tool-call turns (assistant messages whose
  text is empty because they carry tool calls) now preview the call —
  `read {"path": …}` — instead of showing "(empty)"; the popup lists every
  call in the turn, and thinking-only turns show their reasoning.
- Context browser (/context): Enter opens the selected block in a scrollable
  popup, Delete/Backspace (or `d`) removes blocks after a y/n confirmation,
  and Shift+Up/Down selects a range of blocks to delete in one go. Deleted
  blocks stay deleted across later trims and compactions.
- Composer input is a real text editor now: click-to-caret and mouse
  selection, Shift+arrows/Home/End/Up/Down selection, Ctrl/Cmd+A, copy with
  Ctrl+C/Ctrl+Shift+C/Cmd+C and cut with Ctrl+X over the selection, word wrap
  that never splits words, word-wise Ctrl+Left/Right/Backspace/Delete, and
  visual-row navigation across soft-wrapped lines.
- Renamed the project from `phi` to `cozyphi`: module path
  `github.com/alvnukov/cozyphi`, binary `cozyphi`, env prefix `COZYPHI_`,
  and data directory `~/.cozyphi`.
- OpenAI Codex subscription sign-in now uses browser OAuth with PKCE and a
  protected local callback instead of the disabled-by-default device-code flow.
- The plan-gate section of the system prompt now spells out the unapproved
  state: call `plan` with `action=get` before acting, draft or repair with
  `action=update`, then stop until `approved: true`; on a miss, repair the
  plan instead of repeating the identical failing call.

### Added

- The durable plan now gates tool calls once approved: the sidebar shows an
  "approved" checkbox, each step carries a type (explore/edit/run/delegate/
  integrate), and approved plans require `plan_step` on every tool call. The
  initial phase answers misses with corrective feedback and records them to
  `~/.cozyphi/logs/plan-gate-misses.jsonl` for analysis before any hard block.
- A third turn posture, UsePlan, hard-blocks any model tool call whose
  `plan_step` does not name the in-progress plan step; Build and Plan only
  hint. UsePlan is the default startup posture. The mode toggle cycles
  build → plan → useplan with a violet label, and the plan approval
  checkbox is ASCII `[ ]`/`[x]`, toggled with Ctrl+A.
- `/context` opens a full-screen browser over exactly what the model receives
  next: one row per entry with role, token estimate, cumulative share and a
  preview, plus window/threshold numbers. Two actions shape the context —
  compact now, and trim-to-here (`t`, `y` to confirm) which appends a compaction
  note instead of an LLM summary, keeping the append-only audit log intact.

- `/connect` now provides a validated, restart-safe provider catalog with
  pinned subscription integrations: OpenAI Codex through the ChatGPT device
  sign-in flow and Z.AI Coding Plan through its dedicated API key. Both use
  their required pinned protocol and endpoint; Codex preserves stateless
  reasoning/tool state and keeps credential refresh off the UI thread.
- Provider context-overflow rejections now recover instead of failing the turn:
  the session is compacted once and the request retried, matching OpenCode's
  `compactAfterOverflow` behavior. Non-overflow errors keep their fail-fast
  path.
- Prompt history for the composer: Up from the first line recalls the previous
  submission, Down walks back toward the draft (bash-like; multiline drafts
  keep caret movement). History persists in `~/.cozyphi/prompt-history.jsonl`,
  capped at 50 entries, and survives restarts.
- Hover pointer shapes via OSC 22 (kitty 0.31+, ghostty, foot; other terminals
  keep their default pointer): a hand over clickable spots — expandable block
  title rows, tappable footer rows — a text beam over selectable and editable
  text, and a horizontal-resize arrow over the sidebar border.
- Reasoning and compaction summaries render as Markdown (headings, strong/emphasis,
  inline code, and code fences), matching assistant output.
- Codex subscription models expose `:minimal`, `:low`, `:medium`, and `:high`
  reasoning-effort variants through the existing model switcher.
- Z.AI Coding Plan GLM-5.x models expose the same `:minimal`/`:low`/`:medium`/
  `:high` reasoning-effort variants, sent as `reasoning_effort`.

### Fixed

- Approving the plan now hands control to the model instead of bouncing off a
  running reply: approval is accepted mid-run (the loop re-checks the gate on
  its next tool call), and a finished turn blocked on the unapproved plan
  resumes automatically rather than waiting for a manual re-prompt.
- The composer no longer bounces the caret to Home/End when Up/Down has no
  other line to move to, and vertical movement no longer re-opens a dismissed
  slash/mention picker. The composer height is now measured at the width it is
  drawn at, so wrapped input grows upward instead of hiding its first line.
- The context browser now owns the keyboard while open: arrow keys, letters
  and mouse no longer leak into the composer, closing returns focus to the
  composer, and vim navigation works (`j`/`k`, `gg`/`G`, `Ctrl+d`/`Ctrl+u`,
  `3j` count prefixes). Mouse-wheel scrolling no longer snaps back to the
  selected row, and `Shift+G`/page keys reach their handlers.
- ChatGPT subscription model selection now uses the authenticated OpenAI Codex
  `/models` catalog for the connected account instead of remaining stuck on
  four hard-coded entries. The account-bound last-known-good list survives
  restarts, refreshes in the background, and falls back safely when offline.
- Stored provider credentials are now rejected if their endpoint or protocol
  no longer matches the trusted connection contract, preventing a modified
  credential record from redirecting API keys or OAuth tokens.
- Resuming a session (`/resume`, `cozyphi --resume`, `cozyphi run --session/--continue-last`)
  now restores the model the session was using instead of silently falling back
  to the configured default.
- Provider catalog refresh now rejects malformed providers individually, so
  entries with unresolved endpoints (such as Neon's environment template) no
  longer hide otherwise valid `/connect` options.
- Z.AI Coding Plan now uses its dedicated `/api/coding/paas/v4` Chat
  Completions endpoint instead of the incompatible general Responses route.
  Codex OAuth credentials are restricted to their pinned endpoint, preserve
  account metadata across refresh, and propagate data-residency routing.
- Z.AI Coding Plan requests no longer fail with TLS handshake timeouts:
  the shared HTTP client now negotiates HTTP/2 instead of forcing HTTP/1.1,
  which `api.z.ai` drops during the handshake, and transient transport
  failures (TLS/dial timeouts, resets, truncated responses) are retried.
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

- The composer now occupies a single line while empty instead of reserving
  three rows, and still grows with the text up to the existing cap.
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
  `models[].max_output_tokens` in config.yaml (editable in `cozyphi config`,
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


- Slash command set grows to opencode parity: `/new` (fresh session, alias of
  `/clear`), `/compact` (summarize the history now — same pipeline as
  auto-compaction, feedback lands in the transcript), `/export [path]` (write
  the transcript as markdown, default `cozyphi-<session>.md` in the cwd),
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
- `cozyphi run --yolo`: skip all permission checks for one headless run (benchmarks / CI).
- `COZYPHI_PPROF=host:port` serves `/debug/pprof` from the TUI for hang diagnosis.
- `cozyphi -c` / `cozyphi --resume <id>`: start the TUI directly on the newest session for
  the directory, or on a session by id / unique prefix (same flags after `cozyphi tui`).
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
  `--yolo` flag for `cozyphi run`) is now required to skip them; previously the
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
- The fork numbers its own releases: version starts at v0.1.0 (upstream cozyphi
  was at v0.16.0). The update check and `cozyphi update` now target
  `alvnukov/CozyPhi` releases instead of upstream `alvnukov/cozyphi`.

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


- Hooks: `command` UI intents — `status` (footer), `list` (palette page).
- Hooks: session lifecycle events `session_start`, `session_shutdown`, `session_before_switch`.

### Changed

### Deprecated

### Removed

### Fixed

### Security

## [0.15.0] - 2026-08-20


### Changed

### Deprecated

### Removed

- `agent_list` `status` filter parameter (always returns the full list; each row still includes `status`).

### Fixed

### Security

## [0.14.0] - 2026-08-20


- Hook event `command`: `plugin.json` entries register TUI slash commands (`/name` runs `run`). stdout may `submit` a user message or `toast`.

### Changed

- `write` creates or overwrites files (no longer create-only). Use `edit` for surgical changes.
- File tools resolve relative paths against the session cwd and print cwd-relative results (`find`/`ls`/`grep`/`read`/`write`/`edit`). Absolute paths are used internally (including rg/fd) and returned only when the file is outside cwd.
- `find` (formerly `glob`) uses `fd` from `~/.cozyphi/bin` (same as `rg`): respects `.gitignore`, early-stops at limit, optional `limit` arg.
- Renamed directory listing tool `list` → `ls`.

### Deprecated

### Removed

- Built-in `fetch` tool (and `permissions.fetch` config). Use MCP if you still need URL fetching.
- `agent_log` tool (parent agents only get `agent_wait` summaries; job logs remain on disk under `~/.cozyphi/jobs/`).

### Fixed

- `cozyphi update` on Windows: stage the download next to the installed binary (same volume) and fall back to copy when rename still cannot cross drives.
- Assistant fenced code blocks drop the box/`-----` chrome; a muted language caption sits above the highlighted code so mouse selection stays copy-clean.

### Security

## [0.13.0] - 2026-08-18

- TUI hot-reloads the git branch in the path label: switching branches outside the app (another terminal, an editor) refreshes the label automatically.

### Changed
- TUI activity: tool rows keep a 1-cell braille spinner; the footer uses an
  Knight-Rider scan bar so the two don't share the same glyph.
- Tool routing: bash is no longer described as an inspection tool; grep/glob no
  longer nudge `agent_spawn`; `edit.hash` is the 4 hex chars after `#` in
  `@file path#TAG` (leading `#` / full header copy-paste is accepted).
- **Breaking:** hooks are declared in `plugin.json` (one file, many hooks) instead of
  per-directory `hook.json`. Load `~/.cozyphi/hooks/plugin.json` and
  `~/.cozyphi/hooks/<plugin>/plugin.json` (same under the project `.cozyphi/hooks/`).

### Deprecated

### Removed
- Per-hook `hook.json` directories. Use `plugin.json` instead.

### Fixed

### Security

## [0.12.0] - 2026-08-17


- Changelog gate: PRs must update `CHANGELOG.md` (with skip labels / `[chore]`), released sections are protected, and GitHub Release notes are taken from this file.

### Changed

- Hashline `edit` now requires a whole-file `@file path#TAG` (`hash` field) from `read`/`grep`; after a successful edit, re-read before another `edit` on that path. Per-line hashes are 3 letters (a-z) and no longer use digits.

### Removed

- Remove the redundant `agent_task` tool; compose `agent_spawn` + `agent_wait` instead.

## [0.11.0] - 2026-08-16

Baseline release when this changelog became the source of truth for user-visible changes.
Earlier releases are available from GitHub tags only.

<!-- Released section ended -->

[Unreleased]: https://github.com/alvnukov/cozyphi/compare/v0.17.0...HEAD
[0.17.0]: https://github.com/alvnukov/cozyphi/releases/tag/v0.17.0
[0.16.0]: https://github.com/alvnukov/cozyphi/releases/tag/v0.16.0
[0.15.0]: https://github.com/alvnukov/cozyphi/releases/tag/v0.15.0
[0.14.0]: https://github.com/alvnukov/cozyphi/releases/tag/v0.14.0
[0.13.0]: https://github.com/alvnukov/cozyphi/releases/tag/v0.13.0
[0.12.0]: https://github.com/alvnukov/cozyphi/releases/tag/v0.12.0
[0.11.0]: https://github.com/alvnukov/cozyphi/releases/tag/v0.11.0

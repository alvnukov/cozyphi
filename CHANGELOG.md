# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

This project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

- Added: under context pressure the provider view microcompacts old oversized
  tool results — results outside the verbatim tail are replaced with a stub
  naming the tool, the line count and how to recover the output. The session
  log and the transcript stay untouched; editable reads and greps keep their
  anchors verbatim, `/context` reports how many results are elided, and the
  pressure ladder quiets down as the stubbed size drops below the compact
  threshold.
- New `task` tool: the agent works the repository's mcp-ai-helper task
  registry natively. `current` ranks what to do next, `start` names the
  branch and worktree, `done`, `block` and `note` record a dated paragraph on
  the note, and every answer ends with the next move. The tool appears only
  when the main checkout has a registry (`.mcp-ai-helper.yaml` or
  `obsidian-tasks/`), never for sub-agents; `permissions.tasks` (`off`,
  `read`, `ask`, `write`; default `write`) decides how far the model may go,
  the same in every mode including plan, since a note is bookkeeping rather
  than code. The General settings tab carries a row for it, applied live.
  See `doc/tasks.md`.
- An empty `agents:` section in the config no longer disables sub-agents. The
  section was treated as opt-in, so a bare `agents: {}` (or one carrying only
  model overrides) silently turned agents off; now only an explicit
  `enabled: false` does, and clearing `agents.models` prunes the emptied
  section instead of leaving a dead shell behind.
- Approving a plan now starts the session even when every step is still
  pending. Freshly created plans default their steps to pending, and the
  approval resume path only recognized in-progress steps — clicking
  approve on a new plan silently did nothing.
- `/connect` now signs in to a ChatGPT Pro/Plus subscription through the
  browser. OpenAI is one provider with a sign-in step in front of it: the
  browser flow first, a headless device code for a machine with no browser of
  its own, and an API key as its own method on the public endpoint. The
  browser flow is Authorization Code with PKCE over a loopback callback, and a
  reply carrying a state this session did not issue is refused rather than
  exchanged. Esc gives the port back, and the composer stays live throughout.
- The standalone Codex provider is gone. It offered device code alone and
  named a product nobody signs in to, so the subscription it held moves onto
  OpenAI when cozyphi starts: the same models come back under `openai/`, with
  no second sign-in.
- Added: the General settings tab has two new checkboxes — system
  notifications and their sound. Both default to on (notify only when
  unfocused, default sound); changes save to the global config and apply to
  the running session without a restart.
- Security: an MCP server's stderr log is now created 0600 whichever way it is
  written. The append path used 0644, so a log that a failed handshake had
  filled with whatever the server printed, tokens included, was world-readable
  depending on which branch happened to create it.
- Ctrl+U in the message input now discards the line before the caret, the way
  a shell does. It is a line, not the whole draft: a multi-line message has no
  undo, so the chord cannot wipe work the caret is nowhere near. On a
  one-line draft it clears the composer, which is what it was reached for.
- Fixed: a recovered render panic now writes its stack to the debug log
  instead of throwing it away, and the line left on screen says where to look.
  With debug logging off it names the switch that would capture the next one.
  The message also wraps across the pane, so a narrow terminal no longer cuts
  it off mid-sentence.
- `cozyphi run` now explains a failed run with the same classifier the TUI
  uses, so a cause reads the same on stderr as in the transcript. It also
  covers two cases it used to pass over in silence, an unreachable provider
  and an overflowed context. The advice stays headless: a run with no
  composer is told to fix the key in the config, not to press `/connect`.
- Fixed: the warning about a guessed protocol no longer suggests setting
  `provider`, which never took part in the choice. Only `protocol` settles
  it, and that is what the warning now says. The config editor's model
  lookup also honors a row's declared protocol instead of guessing again
  from the model name, so an OpenAI-compatible gateway serving a `claude-*`
  name is listed on the wire format it actually speaks.
- Fixed: overwriting a file with the write tool keeps the permissions it
  already had. Since writes became atomic, the replacement landed with 0644
  no matter what the target was, so rewriting an executable script quietly
  cost it its exec bit. A new file is still created with 0644, and the edit
  tool, which already preserved the mode, now shares the same rule.
- Security: a permission gate that cannot be assembled now denies instead of
  allowing. When both the configured policy and the built-in default failed to
  compile — a workspace root that will not resolve, for instance — the
  controller installed a gate that permitted every request. It now installs one
  that refuses every tool call, names the assembly failure in the refusal, logs
  it, and says so once in a startup toast. An explicitly enabled
  `dangerously_allow_all` bypass is still the only thing that returns
  unconditional Allow. The gate is published atomically, so a policy rebuild
  (`/model`, mode switch) cannot expose a half-written boundary to a run in
  flight.
- Security: write and edit now re-apply the permission verdict to the
  destination while the write is in flight. The gate resolves and judges a
  path when the call is approved, but a directory swapped for a symlink
  between that verdict and the rename used to redirect the file: the
  mutation module resolved what the path pointed at then, not what the gate
  had seen. The destination is now judged again before any parent directory
  is created and once more immediately before the rename, so a redirected
  ancestor fails closed with the physical path named, and nothing is created
  where the link led.
- Hover now shows a visible highlight on interactive transcript rows
  (tool, bash, agent, compaction, thinking titles, expandable status
  lines, tappable list tiles). OSC 22 only reshapes the pointer in
  kitty/ghostty/foot/xterm; iTerm2, Terminal.app, Alacritty and tmux
  users get the same affordance as a quiet background tint on the rows
  a click would act on.
- Fixed: Alt+Enter inserts a newline in the composer instead of
  submitting. Terminals send legacy Alt+Enter as ESC followed by CR;
  the input parser decoded that as a lone Escape plus an unmodified
  Enter, so the composer's Alt-modifier branch never ran.
- Desktop notifications now play a sound — `Purr` on macOS, the freedesktop
  `message-new-instant` hint on Linux — chosen with `notifications.sound`
  (`off` silences it). A turn that ends while a watch is still running sends
  no notification: the watch's next event wakes the session anyway, so the
  ping is saved for when the last watch is gone.
- Fixed: a plan patch batch that replaces the last success criterion
  no longer fails mid-batch. Patch ops apply sequentially and the plan
  is validated as the batch ends up, so a legal batch may cross a
  contract floor on the way; violations are still attributed to the op
  that introduced them.
- Fixed: the wire protocol is no longer sniffed in two places. The
  name/URL heuristic lives in one place (llm.SniffProtocol), still runs
  only when the config declares no protocol, and now warns on startup —
  an OpenAI-compatible gateway serving a claude-* model no longer
  switches to the Anthropic wire format silently; set `protocol` (or
  `provider`) explicitly to pin it.
- Fixed: provider failures now arrive typed instead of as flattened
  strings. API errors carry their HTTP status (llm.StatusError) and stream
  error events carry the error itself, so cancellation stays
  errors.Is-able and the TUI and `phi run` distinguish cancel / rate
  limit / auth failures by code instead of grepping "(429)" out of the
  message text; `phi run` prints a named cause hint on the way out.
- Fixed: an unhandled key can no longer reach the same widget twice. The
  app delivers a key to the focused widget and bubbles the remainder to the
  editor root, whose ladder ends back in the composer — so the chat or
  palette could see one keypress two Handle calls. Harmless today only
  because every mutating branch consumes; EventContext.DeliveredTo now
  marks the first delivery and the composer skips re-delivering to that
  widget.
- Fixed: a PostTool hook's stop signal (stop:true, or exit 2) now actually
  stops the run. It was computed and discarded, so an audit hook could flag
  a bad tool result while the agent kept going. The round's remaining calls
  do not execute, the loop ends with the hook's reason surfaced to the
  user, and the stopped call's result tells the model why the turn ended.
- Fixed: mcp_call is no longer unusable headless. Every server tool still
  asks by default, but a new `permissions.mcp.allow` list in config.yaml
  pre-approves servers or single tools (regex against `server/tool`), which
  keeps MCP working under `cozyphi run` and in sub-agents where an ask
  would fold to a denial. Denial reasons now name the knob.
- Fixed: a panic in a widget's Draw no longer kills the process mid-frame.
  The frame is replaced by an error surface naming the panic and the event
  loop keeps running, so the next event repaints normally instead of
  tearing down hooks and pools with a dead UI.
- The bash tool now distils failures out of long output. Lines a
  toolchain prints only on failure (go test FAIL, compiler
  file:line:col, panic, pytest FAILED, tracebacks, rustc error, make,
  git fatal) are listed up front with their line numbers and indented
  detail, so a failure buried in the middle of a truncated log reaches
  the model instead of the temp file alone; a zero exit code that such
  lines contradict is flagged, since a pipeline reports only its last
  stage. Short output is unchanged, and the TUI keeps showing the plain
  tail.
- Fixed: overlapping line ranges in one edit call are rejected with an
  error naming both ranges, instead of splicing the second edit into
  shifted offsets (or panicking on nested ranges). Adjacent ranges keep
  applying in one call.
- Security: MCP stdio frames are now bounded. A server streaming an
  oversized or unterminated line fails the call at a 1 MiB frame limit
  (naming the server and the limit, never echoing the payload) and the
  transport closes so the next call recovers over a fresh process; the
  per-server stderr log on disk is likewise capped and keeps only the
  newest tail once the cap would be passed.
- Security: an incomplete permission assembly now fails closed. A nil
  bypass gate or one without an inner gate denies with an actionable
  reason instead of silently allowing every request; only an explicitly
  enabled bypass still returns unconditional Allow. The controller's gate
  assembly is covered by a reconfiguration test proving no re-init leaves
  a missing or permissive boundary behind.
- Security: the write tool now lands content through the same guarded
  atomic replacement the edit tool uses, closing the symlink race between
  the permission check and the write. A leaf path that is a symlink at
  mutation time fails closed instead of writing through it, guard and
  preview reads refuse to follow a swapped link (so foreign bytes cannot
  reach diffs, TAG checks or error messages), and a staging directory
  swapped for a symlink while the write is in flight aborts it with
  nothing landing outside.

- Security: the permission gate now resolves symlinks before deciding —
  read/write/edit requests are judged by their physical filesystem target,
  so a symlink (or a symlinked ancestor) that leads outside the workspace
  or into a sensitive path is denied, with the physical path named in the
  refusal. Sub-agent and job workdir boundaries resolve the same way, and
  symlinked workspaces (macOS `/tmp`, `/var`) compare like with like. A
  symlink that stays inside the workspace remains allowed.

- The lsp tool was redesigned around what a model naturally asks, so
  structural questions no longer need a text search. Targeting is
  tolerant: a symbol, a file position, or both are all valid — a
  position picks between several declarations of one name, a qualified
  `Container.Name` is accepted, and a symbol that is merely used (not
  declared) in the file resolves to its occurrence. `file` is optional
  with `symbol`: the name resolves workspace-wide, and an ambiguous or
  unknown name answers with candidate declarations instead of an error.
  Two operations joined the set — `implementations` (interface ↔
  implementations) and `type_definition` — `calls` defaults its
  direction to incoming, and `symbols` accepts `file` plus `query` as a
  filtered outline. Location results now carry the source line as a
  snippet, so an answer rarely needs a follow-up file read. Error
  messages state exactly what is missing; the misleading
  `requires symbol or line+character, not both` refusal is gone.
- The footer now shows the session's live watches — `⏱ N watch(es): label…`
  in both the quiet and the live line, hidden when none run. `Ctrl+W` and
  `/watches` open a full-screen watch browser: each watch's state (running,
  ended, failed), event count and age, `Enter` reads its log, `s` stops a
  live watch after a y/n confirm. The manager's caps and event delivery
  are untouched — the browser only looks and asks.
- The footer's live-watch indicator now breathes: the `⏱` glyph pulses on
  the wall clock while a watch runs, and the call that started a
  still-running watch pulses the same glyph in the transcript instead of
  wearing a checkmark. A click on the indicator folds or unfolds the
  watch's rows in the transcript — its label for one watch, the glyph or
  the count for all of them.

## [0.19.0] - 2026-09-02

- The plan gate's skill-preload interception no longer shows up in the
  feed as a rejected tool call. That refusal only delivers a step's
  skills to the model, which retries the same call at once — the feed
  keeps the `⚙ plan` action row and the retried call, and drops the
  scary `⊘ … (rejected)` row that reported a failure nobody had.
- The sidebar's settings tab gained an `expand edits` switch: on (the
  default), edit diff cards render expanded; off, they render folded to
  their stat line. Unchecking folds every expanded card in the feed at
  once; checking changes nothing already drawn — only future cards
  follow the new default, and a card's own toggle still wins. The
  choice persists across restarts. The old rule — cards open while the
  turn runs and fold when it ends — is gone in favor of this explicit
  switch.
- A plain click folds an expanded feed block. A press inside the body
  still starts a text selection, and a drag that selects copies as
  before — but when the release comes with nothing selected, the click
  collapses the block instead of doing nothing. Title rows keep their
  instant toggle.
- CozyPhi now reads API credentials, models, and MCP servers directly from
  OpenCode as a read-only source. Imported models use
  `opencode/<provider>/<model>` names; CozyPhi MCP entries win name collisions.
  The integration is enabled by default and can be changed under
  `/settings` → General with `opencode.enabled`.

## [0.18.0] - 2026-09-01

- The streaming turn has one live-activity line. While the model works,
  the footer becomes `✻ <model> · Generating… · 42s · ↓1.2k` with a
  right-aligned `Esc interrupts` hint: a breathing glyph, the phase verb
  under a soft letter shimmer — a brightness wave sweeping the word,
  claude-code style, no color change, no blinking — the turn's elapsed
  time and its streamed completion tokens. The footer's scan-bar spinner
  is gone; the only spinner left in view is the active transcript row's.
  The line disappears when the turn ends — the outcome stays in the
  assistant meta row and the turn summary.
- The transcript feed speaks one visual language: a thin role gutter,
  one indent scale, and color reserved for status. Before, an
  assistant-side row started one column in, its tool name wore the blue
  accent whether the call was running or long done, and a diff body sat
  bare on the terminal ground:

  ```
   ✓ edit pane.go +12 −3 ▼
     @@ -40,6 +40,7 @@
     +new line
  ```

  Now every assistant-side block hangs off a `▏` bar in column 0 —
  dimmed for working rows, brighter for the assistant's own text, red
  when the row failed or was rejected — content starts at column 3,
  bodies at column 5, and code-shaped bodies (diff hunks, command and
  tool output) sit on a calm panel backdrop; error rows stay bare so
  the red text is the loudest thing on them:

  ```
  ▏✓ edit pane.go +12 −3 ▼
  ▏ ░@@ -40,6 +40,7 @@░░░░░
  ▏ ░+new line░░░░░░░░░░░░░
  ```

  Static tool names dropped the accent for plain foreground — the
  accent now means "running", nothing else. The gutter is chrome:
  selection copy skips it, user prompts keep their heavy `┃` panel,
  and compaction stays a full-width divider.
- The transcript feed condenses by turn. Turns older than the last two
  fold their working rows — thinking, tool calls, intermediate text —
  behind one muted summary line (`▸ worked 42s · 7 tools · pane.go,
  mapper.go`), keeping each prompt and its final reply in place; a
  click unfolds the turn, and a failed or rejected call, a queued
  prompt or a compaction marker never folds at all. Ctrl+E (rebindable
  as `transcript-verbose`) switches the whole feed to verbose and
  back; Shift+PgUp/PgDn jump the viewport between turns.
- The transcript feed is semantic. An edit or write now renders as a
  diff card: the row names the path and the `+N −M` stats even
  collapsed, the body is the colored hunks, and the running turn's
  cards open themselves and fold when the turn ends (an explicit
  toggle wins over both). Read-only tools summarize what they found
  instead of echoing arguments — `pane.go (641 lines)`,
  `"pat" — 14 matches in 6 files`, `"**/*.go" — 12 files`,
  `pkg (9 entries)` — with the raw body behind Enter, and MCP rows
  name `server · tool`. Failures stopped hiding behind the expand: a
  collapsed tool row shows its first error line and a collapsed failed
  command shows its final output line.
- The plan editor no longer mistakes a session with no plan for a legacy
  one: an empty session opens as a fresh editable draft, and saving it
  creates the v2 plan through the same path the model tool's action create
  uses. Real legacy plans stay read-only with the same message.
- Plan step skills are picked, not typed. In the plan editor the skills
  row of an inject_skill action now opens a multi-select picker over the
  installed skill catalog — Enter/Space toggles a `[x]` mark, `/`
  fuzzy-jumps the list, Esc keeps the checked set — instead of a
  free-text field. Hand-typing stays possible through an explicit
  "other" row, but a name the catalog does not know is never silent: it
  saves with a warning naming it and wears a `⚠` mark in the picker and
  the step's summary row. The planner fills skills in on its own: the
  plan tool's schema now names the installed catalog at every skills
  slot and the plan-mode authoring grammar tells the model to give each
  step its skills from that catalog, so a new plan arrives with
  per-step skill sets without manual entry (unknown names were already
  refused at the tool seam).
- Global hotkeys are now rebindable through a `keybinds` config section:
  command id → chord (`plan-editor: Ctrl+G`; `none` unbinds; a comma
  separates synonyms). One binding table in `internal/tui/keys` drives
  the dispatch in the editor, the footers, the help screen and the
  palette's shortcut column, so an override changes the behavior and
  everything that advertises it together. The section is validated at
  load — an unknown command, a malformed chord, or two commands on one
  chord fails the start with a message naming the conflict. The
  rebindable commands: `help` (F1), `palette` (Ctrl+K), `settings`
  (Ctrl+,), `plan-editor` (Ctrl+P), `plan-focus` (Alt+P),
  `sidebar-toggle` (Ctrl+O), `plan-approve` (Ctrl+A), `plan-details`
  (Ctrl+D), `copy-last` (Ctrl+Shift+C, Cmd+C).
- The Allow-All options now say what they actually do: an explain row
  under the options follows the selection — "Allow All for This Session"
  admits it stops asking for every tool until CozyPhi exits, and "Allow
  All for Every Session" names the exact rule and file it writes
  (permissions.dangerously_allow_all in the global config). The
  persistent grant is no longer silent: choosing it arms the standard
  y/n confirmation naming the file, only y writes, and a toast reports
  where the rule landed — or the error if the write failed.
- The permission ask now shows the whole request instead of a hard clip:
  a long command collapses to twelve lines with a marker naming `v`, and
  `v` expands it into a scrollable window (`↑↓`/`j`/`k`, the wheel; the
  first `Esc` folds it back, digits and `y`/`n` still answer while
  reading). Edit and write asks carry a colored diff of the exact change
  being approved — rendered by the same engine as the transcript diff —
  instead of a bare path list. On a short terminal the detail is what
  gives way: every option and the hint stay reachable at any height.
- The modal asks (permission, continue, question) now own the mouse the
  way they own the keyboard: a click on an option selects it, a click on
  the selected option activates it, a description row counts as its
  option, and the wheel steps the choices wherever the pointer is. A
  click that fell through the modal used to resize the sidebar or scroll
  the transcript mid-answer; now every stray mouse event dies at the
  modal.
- A failed run now explains itself instead of dumping the provider's raw
  error: the transcript entry opens with the cause and the fix — bad
  credentials point at /connect, a rate limit says how long to wait, a
  context overflow points at /compact, an unreachable host names the
  network — keeps the raw error as the detail, and reminds that ↑ in the
  composer recalls the prompt for a retry. Error entries carry a
  distinct "✕ run error" marker so they no longer read as an assistant
  answer; compact failures get the same classification.
- Toasts queue instead of overwriting each other: one message holds the
  slot for its full lifetime and the next waits its turn, so an error is
  never cut short by the info toast behind it. The last twenty
  notifications are kept and readable from the palette — Ctrl+K →
  "notifications recent".
- The command palette (Ctrl+K) now lists every immediate builtin — help,
  the context browser, connect, compact and sessions rode only the slash
  picker before — and prints the global chord next to a command that has
  one (F1, Ctrl+,, Ctrl+P, Ctrl+Shift+C). The chord spellings come from
  the keys catalog, so the palette and /help cannot disagree.
- The context browser (/context) answers `/` and `.` too: `/` fuzzy-jumps
  over the entries by kind and preview, and `.` lists the selected
  entry's commands — view, trim, delete, compact, refresh — each naming
  its chord. The letters left the footer for the menu, so the footer
  reads `↑↓/j/k move · Shift+↑↓ select · Enter view · / jump · . menu ·
  Esc close`.
- The settings modal answers `/` and `.` the way the plan editor does:
  `/` opens the fuzzy-jump strip over the active tab's list and `.` an
  action menu naming each command's chord (`Apply changes (Ctrl+S)`,
  `Next tab (Tab)`). The pair now lives in the shared browse kit, so
  every list that adopts it behaves identically by construction.
- The plan editor answers `/` and `.`: `/` opens a fuzzy jump that moves
  the selection to the tightest match as you type — the strip counts the
  matches, `↑↓` cycle them, `Enter` keeps the landing, `Esc` restores
  where you were. `.` opens an action menu for the selected row, each
  command naming its chord (`Move step down (Alt+↓)`), with undo and
  redo appearing when there is history to walk; the footer got shorter
  because the chords now live where they are discovered.
- Plan fields are edited in place now: `Enter` opens an editor strip
  along the bottom of the panel — a rule row naming the field and its
  length budget, then the text, growing with its content up to six lines
  — instead of a popup that covered the plan. The list stays visible
  above it with a passive marker on the edited row, so an edit never
  loses its context.
- The plan editor goes two-pane on wide screens (a panel of 86 columns or
  more): the plan list keeps the left column and the right column expands
  whatever is selected — a field in full with its length against the
  limit, a step's detail form, or the plan overview. Opening a step keeps
  the list visible with a passive marker on the open step. Every list now
  keeps its own cursor, so `Esc` from a step's details lands back on that
  step and a choice list returns to the row that opened it — the
  selection no longer snaps to the top. Focus follows the mouse: click a
  list row to act there, click a preview row to open the details on it,
  and the wheel scrolls the pane under the pointer. Narrow terminals keep
  the single-column layout unchanged.
- The plan editor has undo and redo: `Ctrl+Z` takes back one logical edit
  (a saved field, a toggled flag, a reorder — not one keystroke), `Ctrl+Y`
  brings it back, and undoing to the baseline clears dirtiness, so `Esc`
  closes without the discard question. Every changed row wears a `●`
  marker — in the step list and in the step details, down to the exact
  action aspect that changed — and the header counts the unsaved edits:
  the total is the number of dots. History that would lie after the plan
  moved underneath (a rebase) is dropped rather than replayed, and a
  choice list refuses undo with a hint instead of mutating under a picker.
- Paging the transcript keeps one row of overlap, like every other scroll
  view, so the seam between screens stays readable.
- The sidebar plan pane speaks the standard motion dialect once focused:
  counts (`3j`), `gg`/`G`/`12G` jumps and page keys all work, and the
  viewport now follows the selection — arrowing through a long plan used
  to walk the cursor out of view. `Space` opens the model picker like
  `Enter`, and in the picker `Space` commits like `Enter`. The standing
  hint row moved into the bottom border and follows whoever owns the
  keyboard — the idle sidebar, the focused plan, or the picker — which
  also gives the step list one more visible line.
- The ask modals answer the standard keys everywhere: `Space` takes the
  highlighted option like `Enter` in the permission ask, the continue
  prompt and the question ask, and a key the question ask cannot use now
  says which keys work — the same warning the other asks already gave —
  instead of being swallowed without a trace. The help screen documents
  the whole set (`j/k`, `h/l`, digits, `Space`, and that the options wrap).
- Moving a plan step is `Alt+↑↓` now, the same chord editors use to move a
  line. It was `Shift+↑↓` — but `Shift+↑↓` extends the selection in the
  context browser and in every list outside this TUI, so in the plan editor
  it now explains itself and points to `Alt+↑↓` instead of silently
  reordering the plan. The footer, the help screen and the standard all
  carry the new chord.
- The plan editor speaks the full motion dialect: counts work (`3j`, `12G`)
  in the step list, the step details and every choice list. Its
  confirmations follow the standard too — any key that is not `y`, `n` or
  `Esc` withdraws the question and acts, so a delete question can no longer
  swallow your keys, and a stale `y` can never delete what the cursor left
  behind.
- The settings modal moves like every other list now: counts work (`3j`),
  `gg`/`G` jump to the edges (a lone `g` used to jump on its own; it now waits
  for the second `g`, the same as everywhere else), and each tab keeps its own
  place — wheel-scrolling one tab no longer forgets where the others were, and
  the window stops snapping back to the selected row on every repaint.
- The context browser's confirmations behave by the standard now: `Esc` backs
  out one level — it cancels an armed trim/delete question instead of closing
  the whole browser — and any other key withdraws the question instead of
  leaving a stale `y` waiting while you move around. A trim confirmed with `y`
  fires on the row it named when it was armed, not on wherever the cursor sits
  now. The block viewer popup speaks the full dialect too: counts, `gg`/`G`,
  `Ctrl+U`/`Ctrl+D`, `q` to close, and the wheel covers three lines a notch.
- The help screen now moves like every other list: counts work (`3j`, `5G`),
  `gg` and `G` jump to the edges, `Ctrl+U`/`Ctrl+D` scroll half a screen, and
  the wheel covers three rows a notch instead of one. A single `g` no longer
  jumps to the top — it waits for the second `g`, the same as everywhere else.
  Behind it sits a shared motion engine (`internal/tui/browse`) and a written
  interaction standard (`internal/tui/DESIGN.md`) that the remaining panes
  will be ported onto, so the whole TUI answers the same keys the same way.
- The plan editor no longer throws away your edits when the agent changes the
  plan underneath it. `Ctrl+S` used to come back refused with a stale-revision
  error, and the only way out was `Esc` — discarding everything you had just
  typed. The draft is now merged onto the newer plan and saved. When a field
  moved on both sides, the editor stays open on the newer plan and names what
  it took, so the next `Ctrl+S` is a decision rather than a rewrite.
- The plan editor's keys now say what they do. `Ctrl+S` inside a field popup
  saves that field and stops there — it used to also write the whole plan and
  close the editor, from a popup that only ever advertised Enter, Esc and a
  newline. Committing the plan stays on the step list, where the footer
  promises `Ctrl+S`; that footer now also lists `Del`, which the list always
  accepted and never mentioned. `Backspace` has stopped deleting rows behind
  the footer's back — in a list where it reads as "go back" it now says which
  key deletes.
- `read` in view mode opens each page with a stats header, `@read path (N lines,
  size, showing A-B)`: one call now answers how long the file is and which lines
  came back, so pagination needs no scouting read. Files streamed past the
  in-memory cap report size without a line count; editable reads keep their
  `@file` header unchanged.
- `/help` (or F1) opens a full-screen list of every keyboard shortcut, grouped
  by where it works — the composer, the transcript, the plan editor, each ask
  and each full-screen pane — including the keys that were never advertised
  anywhere, like a digit picking an option in an ask. The footer hint rows now
  come from that same table, so what a pane promises and what it does can no
  longer drift apart, and they read the same way everywhere.
- Every prompt inside a modal — deny-with-feedback, a question's custom answer,
  the connect provider filter — now edits like a real field instead of only
  appending: the caret moves with the arrows, Home and End, and Ctrl/Alt widen
  Backspace, Delete and the arrows to whole words. Each accepts a paste, which
  the asks used to drop on the floor, and each scrolls on its row, so a value
  longer than the panel no longer carries the caret off the end and looks like
  typing has stopped working.
- The approval and continue asks answer the keys they advertise: a bare digit
  picks its option (no Alt), `y` approves and `n` denies, `j`/`k` walk the
  options as a ring, and a key that does nothing now says so instead of being
  swallowed silently. `Esc` is labelled "deny", which is what it does.
- An approval ask shows every path it would touch, and up to twelve lines of a
  command instead of three, so the redirect at the end of a heredoc is visible
  before approving. Longer detail is elided with a count, and the options are
  kept on screen even when the panel is short — an ask nobody can answer used
  to stall the run. The panel is also measured at the width it is drawn at, so
  the sidebar no longer pushes the last options off the bottom.
- Ctrl+C interrupts instead of killing the session: it declines a pending ask,
  then cancels the shell command or agent run, then clears an unsent draft.
  Only when nothing is left to interrupt does it arm the exit — a hint says so,
  and a second Ctrl+C within two seconds quits.
- `agents.models` pins are resolved by one resolver shared by the TUI and
  headless `cozyphi run` — same pin rules, same warning list, differing only in
  the catalog each can see (the TUI adds connected-provider models, which it
  alone has).
- Desktop notifications default to `unfocused` instead of `always`, and a
  terminal that never reports focus keeps notifying (focus reports are trusted
  only once one of them says the terminal lost focus). A sender failure — which
  switches notifications off for the session — now raises a toast instead of
  only a debug-log line.
- Editable `grep` anchors are now reported structurally by the grep tool
  itself instead of being parsed back out of its rendered text, so a change to
  the output format can no longer silently stop authorizing edits. Anchors cut
  by the output cap stay unauthorized, as before.
- An `edit` that did not change the file keeps its authorization: a wrong TAG,
  a mistyped anchor, or a rejected range can be corrected and retried without
  another `read` with `mode:"edit"`. An edit that applied still ends it, and
  the ledger now bounds what it tracks (16 file snapshots, 4 reads each).
- Plan-step skills are preloaded once per session: a skill named by several
  steps sends its body the first time and a one-line reminder afterwards, and
  a reminder no longer refuses the call that started the step (only unseen
  guidance does). Compaction clears the record, so a body summarized away is
  sent again in full.
- Fixed: a plan step that names a skill the catalog cannot supply (a typo, or a
  skill removed since the plan was authored) no longer receives an empty
  heading under a "no read call needed" banner — the missing names fall back to
  the read-the-SKILL.md instruction, and the miss is logged.
- Fixed: a `read` in view mode no longer loads the whole file into memory. A
  file larger than 8 MiB is windowed off the disk one page at a time (the page
  cap is unchanged), so reading a large log costs the page, not the file;
  `mode:"edit"` still refuses such files because the anchors need a whole-file
  hash.
- **Breaking:** `read` now defaults to a numbered `N|content` view without
  hashline overhead. Use `mode:"edit"` to receive one-shot editable
  `@file path#TAG` / `N#HASH|content` anchors; `grep` remains an editable-anchor
  source, and `edit` rejects view, foreign-session, replayed, and stale anchors.
- TUI sessions start on the last model used, not the config default: the
  active model name is remembered in global UI state and restored on a fresh
  start (`COZYPHI_MODEL` still overrides it; headless `cozyphi run` keeps the
  config default).
- Per-role sub-agent models: `agents.models` in config.yaml pins a configured
  model per role (explore|worker|review), editable in the new Agents tab of the
  settings modal (bulk "all roles" picker plus per-role pickers, "(inherit
  session model)" clears the pin and applies without a restart). Unknown role
  keys fail the config load; unknown model names degrade to inheritance with a
  startup/apply warning, and the `agent_spawn` transcript row names the model
  it actually used.
- Fixed: `agents.models` pins now resolve against connected-catalog models
  (`providerID/modelID`, e.g. a model chosen from the `/connect` catalog) as
  well as the static `models:` list. Previously a pin named a catalog model,
  which the settings picker offered, but resolution only searched static
  config models — so the pin warned "unknown model (inherit)" and silently
  degraded to the session model. Headless `cozyphi run` still resolves against
  static config models only.
- Plan-injected skills now arrive as complete plain-text `SKILL.md` bodies before
  the first working tool dispatch. If a tool call starts the step, the runtime
  installs the context, refuses that call, and asks the model to retry it.
- Memory seams understand JSON-escaped paths: on Windows, a tool call naming
  the memory directory used to go unnoticed (every separator arrives doubled
  in tool-call arguments), so rewrites in place never reached the next turn
  and recall queries ranked wire-form paths that match nothing.
- Desktop notifications when the model stops or asks for input (permission,
  continue, question): the new `internal/notify` package drives `osascript` /
  `notify-send` argv-only, tracks terminal focus, and the `notifications.mode`
  config knob gates it (`off|always|unfocused`, default `always`).
- CI hardening: tests run under `-race` on Linux and macOS, lint and format
  checks are separate jobs, coverage lands in the job summary, every job has
  a timeout, main runs are never cancelled by a newer push, and the
  golangci-lint version is pinned once in `.golangci-lint-version` (CI and
  `make lint-install` share it). The test matrix is Linux + macOS; the
  Windows leg is gone — OS-specific test semantics kept it red at real
  cycle cost, while the suite stays portable.
- Session plan tests compare timestamps through the same JSON round-trip the
  durable file performs, fixing false failures on UTC hosts.
- Plan steps can inject skills: the model authors `inject_skill` actions
  through the plan tool, users toggle each skill from the plan sidebar
  (circle per skill, one click before or after approval), and off marks
  survive plan reloads and planedit authoring — shown as `name (off)` in the
  step action row. Disabled names ride along in `DisabledSkills` and only
  reach the session when their skill is on.
- Agent loop counts compact-strike pressure per tool round instead of per
  finished turn: a runaway tool loop escalates (soft → hard gate → stop) inside
  a single turn, gets one final compaction offer round, and — without a
  compaction — ends with `ErrCompactionRequired` instead of burning requests;
  a landed compaction re-arms the ladder and the turn continues. An inference
  whose estimated context exceeds the model window is never sent at all.
- New integration scenario gate (`internal/planscen`) walks ten deterministic
  authoring scenarios — from trivial tasks to a mid-plan material supersede
  that still closes as success — through the real permission gate, approval
  and lifecycle path, with no gate mocks.
- Plan telemetry gains bounded authoring-friction counters (drafts by
  authoring policy, approval latency buckets, material reapprovals, patch
  retries, completion outcomes); the snapshot stays uint64-only, so no plan
  text or free-form label can enter it.
- Plan defaults gain `authoring_policy`, a closed selector for the plan-mode
  authoring grammar: `adaptive-minimal` (default) or `legacy`, which restores
  the pre-grammar appendix byte for byte; invalid values fail config load.
- Plan-mode prompt teaches an authoring grammar: obligations to workstreams,
  dependency and uncertainty naming, evidence boundaries, the smallest
  complete bespoke plan, least sufficient capability type, and a model-side
  self-check, bounded to roughly 130-170 tokens by tests.
- Plan patch gains `supersede_step`: a step whose capability must change
  mid-plan turns superseded — terminal, its evidence and audit trail intact,
  never blocking a success close — while a fresh pending replacement takes
  its place; reapproval follows the contract change between the pair, not
  the swap itself.
- Plan sidebar scrolling: scrolling to the bottom now reaches the last
  plan line (the viewport height the renderer uses is also what the
  clamp uses), and operational updates — status and revision ticks —
  no longer snap the viewport back to the top; only material plan edits
  reset the view.
- Settings pane skills editing for plan actions is a picker over known
  skills instead of typed names: activating a skills row lists the
  installed skills with [x]/[ ] toggles, Escape collapses it.
- Every model picker (Ctrl+K palette, sidebar overlay, settings list) pages
  with PageUp/PageDown and takes vim navigation: Ctrl-chords and letters page
  the palette, the sidebar and settings lists move with j/k, g/G, and Ctrl+D/U
  steps the settings list by half a viewport.
- Ctrl+K palette and the model pickers now rank by usage: accepting a submenu
  entry credits the parent row, typed queries blend usage weight into the text
  score, and every model picker (palette, sidebar, settings) shows one shared
  ordering fed by the same history. Ctrl+K rebuilds the root list on each open.
- Selecting text in the input line with the mouse now copies it to the
  clipboard on release, matching the transcript's copy-on-select; a plain
  click still only moves the caret.
- Compaction reminders and the standing prompt policy now direct the model
  to put what must survive compaction into its last assistant message —
  recent messages survive compaction verbatim — instead of the plan's
  workingContext, which nothing reads back after compaction.
- Compaction reminders now leave a visible trace for the user: every
  context-pressure strike and plan compact nudge publishes a `CompactNotice`
  session event that renders as a local `context` transcript row (error-tinted
  once the ladder reaches the hard strike), and the reminder text itself
  demands a one-line acknowledgment to the user — silent compliance looked
  like an ignored message, while watch events were always reacted to visibly.
  The system prompt now also carries the same policy as a standing
  instruction: on a compaction reminder, put what the work still needs into
  the last assistant message (recent messages survive compaction verbatim),
  say so in one line, then call the context tool to compact.
- Step-type model pins set in the settings pane now persist. `Compile` and
  `Policy.Defaults` rebuilt `TypeDefaults` without the `Model` field, so every
  settings apply silently erased the pin from `plan.defaults` and the pane
  snapped back to `(session default)`.
- A plan compact action's advice now reaches the model in the tool result of
  the very call that fired it (its settle transition or the plan tool's own
  action) instead of riding the next user prompt one boundary late; the
  queue drains in-call, so the next prompt no longer repeats it.
- `plan_step` is now a required property on gated tools' schemas. Providers
  sample tool arguments against the schema, and an optional `plan_step` was
  a property the sampler dropped at will — the plan gate then reported a
  miss for a step the model did name (most often on the larger schemas of
  grep/find/ls). Voluntary bindings on additionally-exempted tools stay
  optional; the exempt utilities never carried the property and still do
  not.
- The context-pressure compact reminder escalates instead of firing once:
  every agent turn that ends over the threshold without a compaction
  re-queues the reminder, and the wording hardens on the third — from that
  strike the executor refuses every tool but the context tool, with the
  refusal itself naming the way out. A further uncompacted turn stops the
  model loop entirely (`ErrCompactionRequired`; run /compact to release),
  and landing any compaction — model- or user-initiated — resets the
  ladder. Context pressure no longer stays silent when a plan compact
  action parked its own nudge earlier in the turn: the fresher fact wins.
- Plan Defaults settings now edit default plan actions, closing the gap
  between the two action scopes and their configuration. `plan.defaults`
  gains plan-scope `actions` (plan_start / plan_end) and per-type `actions`
  (step_start / step_end); new plans inherit the plan-scope list and their
  steps the type's list when their author defines none, insert_step seeds
  the same way, and explicit author lists win untouched. The Plan Defaults
  tab renders both blocks: add/remove, click cycles the event within its
  scope and the action type (compact ↔ inject_skill, flipping to compact
  drops skills), skills edit as comma-separated text, and validation reuses
  the session normalizer at compile and apply time.
- Plan compact automation works again: crossing the context threshold no
  longer auto-compacts mid-step. The engine now injects a model-facing
  reminder — record must-keep state durably, then call `context compact` at
  a safe boundary — through the watch-reminder channel, debounced to one per
  turn and reset after a compact. The threshold is configurable as
  `compaction.reminder_tokens` (top-level YAML key, 0 = harness default)
  with a “Compact reminder threshold” field in sidebar Settings → General;
  applying it live-pushes the value to the running engine.
- Live streaming names its model everywhere: reasoning rows and the
  assistant row read `<model> · thinking` with the wave animation while the
  turn streams, and the footer loader reads `<model> · <phase>` — the
  engine's live model beside the current phase label — through every run
  phase (awaiting reply, generating, calling tools, compacting), instead of
  dropping to a generic label while tools run or before the first token.
  The sidebar status shows the engine's live model, with plan-step badges
  resolving against the session default.
- Exempt work tools (`read`, `grep`, `find`, `ls` — the configured
  `additional_exemptions`) now honor a voluntary `plan_step`: the binding
  starts a pending step before dispatch — its model pin and `step_start`
  actions fire — and the call's evidence files there, so a step built entirely
  of exempt calls no longer silently skips its start automation. An exempt
  call never misses or denies: a `plan_step` that names no active step passes
  with a corrective note.
- Completing a still-pending step that owes start automation (a model pin or
  `step_start` actions) is now refused on both completion doors — the `plan`
  tool's `complete` and the `_plan` envelope — with an error naming the step
  and how to start it; plain pending steps still complete in one call.
- The session sidebar leads with the session model — default included
  (`(unset)` instead of a blank) — and every plan step carries a model badge
  (`◇`) with its effective model: a step pin, else the step-type map, else
  the session default. Skills left the sidebar: the plan-settings tab opens
  with a `skills: …` enumeration of the names an `inject_skill` action can
  take, and an `inject_skill` chip in the plan reads
  `skills: tdd, code-review@step_start` instead of `inject_skill: …`, so the
  same vocabulary names skills everywhere the model or the user meets them.
  The editor resolves skill names once per session instead of re-walking the
  skill tree on every frame.
- The `plan` tool's model-facing surface no longer carries the plan's
  automation fields: `model`, `actions` and `modelsByType` are absent from
  the schema, refused with a dedicated `human-only` error in `create` and
  `patch`, and sanitized out of receipts and `get` views. The plan UI and the
  engine's step-model resolution are unchanged.
- The `plan` tool now treats `action` as the authoritative payload discriminator:
  provider-materialized defaults from other actions are ignored after strict unknown-field
  decoding, patch revisions and lifecycle retry IDs come from harness state when omitted,
  and new steps default to pending without model-supplied lifecycle state. The plan-gate
  prompt now advertises the minimal v2 create call instead of the incompatible legacy
  steps-only form.
- The plan pane no longer swallows control keys while typing: real focus
  back in the composer releases the plan pane's keyboard mode (and closes
  its model picker), so ↑↓/Enter/Esc/m reach the composer instead of the
  plan steps. `alt+P` still hands the keys to the plan pane.
- Sidebar plan action chips list their parameter: a chip reads
  `compact --all@plan_start` or `skills: tdd, code-review@step_start`
  instead of an anonymous `compact@plan_start`, so the chip says what fires
  and with what payload.
- The plan editor names the step being edited everywhere: the browse list
  shows step ids, the detail screen's title and heading carry the step's
  position and id (`Step 2/3 · beta`), and text popups read
  `Edit beta · content`, so editing one step can no longer be mistaken for
  editing them all. Editing a step compiles to exactly one `update_step`
  patch addressed to that step.
- Interactive asks (permission, continue, question) no longer time out: the
  shared ask helper waits indefinitely on the reply channel, so the model idles
  until the user answers instead of resolving to "Unanswered" after 120s.
- The sidebar plan pane renders the full Plan v2 contract: goal above the
  steps, blocked reasons with resume conditions in the expanded view, and a
  Ctrl+D flip between brief and rationale (why/done_when/outcome/evidence
  refs) sharing one viewport. A material revision that revokes approval now
  shows the bounded diff against the last approved snapshot (`reapproval:
  N changes` plus `target.field` lines, never replaced prose), closed plans
  show a `closed: <result>` row that a reopen removes, and `session.MaterialDiff`
  is the one exported table both the approval decision and the diff view read.
- Plan v2 gains a bounded observability budget: a new `internal/plantel` tracker
  counts misses, material revisions, approval churn, transition conflicts,
  idempotent retries, standalone starts, plan-only rounds, projection bytes,
  completions without evidence and archive latency — uint64 counters only, never
  plan content. The snapshot surfaces via `plan get view=telemetry` and degrades
  to zeros when no sink is wired; recording never blocks the plan loop.
- Durable-plan text is now sanitized on every write door: a new `internal/redact`
  pack masks known secret shapes (AWS/OpenAI/GitHub keys, bearer tokens,
  credential-shaped assignments) in create/patch/transition/settle, attempt
  summaries, diff details, the legacy update path and evidence refs; control
  characters are rejected in plan prose and stripped from attempt summaries; the
  patch path enforces the serialized plan budget; JIT approval questions quote and
  mask model text; and the plan tool contract drops the stale injected-snapshot
  claim in favor of a secrets/chain-of-thought/raw-logs ban.
- Completing the last working step of a v2 plan now auto-finishes and archives it
  in the same write: a `complete` carrying a plan result — on the plan-tool road
  or the `_plan` settle road — closes the plan with no extra model round, the
  repeated settle is replay-idempotent, the closed plan serves a bounded terminal
  projection instead of the step list, and plan-level `reopen` (a reason, no id)
  clears the terminal result while the audit stays readable via `view=full`.
- Plan prose caps doubled to 512 characters (step content, why, done-when,
  outcome, risk, note, evidence, blocker, resume-when, transition reasons,
  success criteria and constraints); the v2 serialized plan budget grew from
  48 KB to 96 KB to match. Loading older snapshots stays compatible.
- A plan step sent without a status now starts pending instead of rejecting
  the whole plan write with `invalid status` — drafting sends contract fields
  only; genuinely unknown statuses still fail closed.
- Working tool calls can now settle the plan in the same round: a `_plan`
  envelope on any gated call completes the previous step (outcome/evidence),
  swaps the working context and starts the named step as one atomic,
  idempotent plan write before dispatch. The envelope is harness-owned — it
  never appears in tool schemas — tool arguments are validated against the
  schema the model saw, an invalid envelope rejects the call with no partial
  plan mutation, and a runtime tool failure leaves the completed step
  completed. This removes the plan-only model round between adjacent working
  steps.
- The plan hint in the system prompt is now a constant presence marker instead
  of a per-write line (revision/steps/remaining/approval). The old line rode
  the tail of the system prompt and changed on every plan write — including
  the attempt record the gate leaves on each gated call — which broke the
  provider's prefix cache at that point and re-billed the entire conversation
  history at full input price on every plan mutation (observed as the cached-
  tokens label flickering between the post-compaction floor and full hits).
  Volatile plan state still reaches the model through tool results and plan
  tool responses, which persist in history and keep the cache prefix intact.
- New "Plan" checkbox in the sidebar settings tab switches the plan feature
  fully on or off, live and persisted: it removes the plan tool from the
  model's toolset and the plan-gate/hint blocks from the system prompt, hides
  the sidebar plan pane (Ctrl+A becomes inert), the `/plan` command, the
  plan-editor palette entry and the plan mode hop in the posture cycle — and
  disabling keeps the durable plan itself, so re-enabling restores everything.
- Provider requests no longer carry the current plan as a synthetic `plan`
  tool round appended to the messages: the plan reaches the model through the
  system prompt only (plan-gate block and plan hint), and every request is
  exactly the durable session history. The `plangate.PromptSnapshot`
  renderer was removed with it.
- Steps marked `jit: true` now require their own user approval. Plan approval
  covers the contract, not the irreversible effect such a step names: its
  first gated call stops after the permission gate and hands the user the
  exact step, action and declared risk. Approval is remembered durably,
  pinned to the plan's contract epoch and the step's stable id — operational
  writes (status, attempts, evidence) keep it, any material change, a
  reopened-with-new-action step, or a different step expires it, and
  re-approving the plan revives nothing. A denial names the step, action and
  risk without leaking model context; without an interactive ask handler
  (headless runs) the call fails closed; steps without the marker behave
  exactly as before.
- Plan `create` and legacy `update` now accept provider-materialized zero
  defaults for fields owned by other plan actions; meaningful misrouted fields
  remain rejected.
- The plan editor is now a compact keyboard-first settings browser with wrapped
  cursor-aware text popups, step detail forms, configured-type step creation,
  confirmed deletion, reordering, and atomic revision-guarded patch compilation.
- The `<current-plan>` snapshot injected on every inference and the plan
  tool's `get` answer are now one bounded projection instead of the full
  canonical snapshot: goal, approach, success criteria, constraints, working
  context, progress counts, the active and blocked steps in full (with their
  citable attempts), collapsed completed outcomes (id + outcome), and the
  nearest pending steps. Audit history, events, and evidence prose stay
  durable and are served only by `plan {"action":"get","view":"full"}`. The
  projection obeys one fixed byte budget with a documented truncation
  priority — completed history and pending steps shed first, directive tails
  and step enrichment next, header prose last — backstopped by a byte-floor
  escape pass that keeps the budget a hard invariant even for wide-rune
  plans; whatever is dropped is counted in an `elided` block. The wrapper
  marks all field values as untrusted, model-authored data, and JSON escaping
  keeps plan prose from closing it.
- Every accepted plan-gated tool call now leaves a bounded attempt record on
  the step it advanced: call id, tool, terminal status (success, failed,
  canceled, or lost when the result could not be delivered) and a
  harness-truncated summary. Raw tool output never lands in the plan — the
  transcript holds it by call id — and a failed or canceled attempt moves no
  lifecycle state. `complete` evidence refs may cite a recorded successful
  attempt as `call:<callId>`; a ref naming a missing, failed, or foreign
  attempt is refused. Re-reporting the same call id updates the one record in
  place instead of duplicating it, and the per-step history is a bounded tail.
- A gateable tool call that names a still-pending approved-plan step by its
  stable id now starts the step itself — no separate plan call between
  approval and the first tool. The call must first clear the plan-type and
  permission gates; a start that lost a race with another call naming the
  same step counts as done, a runtime tool failure leaves the step
  in_progress and retryable, and the visible tool list now includes what
  eligible pending steps allow, so the first call of a step can happen.
  `plan_step` is the stable step id; the legacy 1-based number keeps working
  and is answered with a deprecation note.
- The durable plan is now viewable and editable in the TUI. A modal pane
  (Ctrl+P, `/plan`, or the command palette) renders the goal, approach,
  success criteria, constraints, working context and every step with its
  fields; editing a row builds a draft whose diff applies atomically through
  the revision-guarded plan patch path. Stale revisions and legacy plans
  refuse loudly instead of silently dropping edits.
- Plan approval now follows one material-change table. Goal, approach,
  success criteria, constraints, step action/type/done_when/risk/just-in-time
  posture, and added, removed, or reordered steps revoke approval and answer
  with a compact material diff addressed by stable step ids; working context,
  wording-only why updates, notes, status, outcome, evidence, and blockers
  keep it. The `create` and `patch` answers carry the diff, transitions never
  touch a material field, and approval stays user-owned — the tool input has
  no `approved` field to send.
- Switching to an Anthropic-shaped partner that runs in thinking mode (e.g.
  DeepSeek) no longer fails the first call: the previous turn's reasoning is
  passed back as a `thinking` content block instead of being dropped.
- The plan tool gains validated step lifecycle actions: `start`, `complete`,
  `block`, `resume`, `cancel`, and `reopen` move one step by stable id through
  the session's state machine. `complete` requires an outcome plus evidence
  (item, refs, or an explicit `no_evidence_reason`); `block` records its
  blocker and resume condition; `cancel` and `reopen` require a reason. Patch
  cannot touch status; in a v2 plan, after create, status moves only through
  the lifecycle actions. Every transition appends a
  bounded audit event, replays recorded results for a repeated `mutationId`
  without a new revision or duplicate evidence, and refuses forbidden moves
  with the current status and the allowed actions.
- The plan tool gains `action=patch`: an atomic batch of domain-specific
  operations (`set_plan_fields`, `replace_context`, `update_step`,
  `insert_step`, `remove_step`, `reorder_steps`, add/update/remove constraint
  or success criterion) applied all-or-none against `expected_revision`,
  addressed by stable step ids — never array indexes or JSON Pointer. Scalar
  slots follow patch semantics (absent keeps, value replaces, JSON null clears
  optional fields only); a stale revision reports the actual one; a failing
  batch leaves the plan, its revision, and its approval untouched; the answer
  is the changed delta, not a snapshot.
- The plan tool speaks a discriminated contract: `action=create` stores a full
  v2 work contract (goal, approach, success criteria, per-step id/why/done_when)
  as an unapproved draft, `action=get` returns a compact active view (explicit
  `view=full` for the canonical snapshot), and the legacy steps-only call keeps
  working on a marked compatibility path. Misrouted or incomplete calls now
  error with the missing field and the allowed action.
- Slash command failures now surface exactly once: `Run` returns errors and
  the dispatcher toasts them, so argument mistakes warn gently while real
  failures (a rejected model switch, say) stay up as errors instead of being
  dropped silently or announced twice.
- Ask overlays (permission, continue, question) size themselves by the rows
  they actually render — wrapping included — instead of counting newlines, so
  long commands or option lists no longer truncate their own options out of
  reach on narrow terminals.
- The mention/slash picker's selection bar and the transcript's block-copy
  highlight now come from the active theme instead of hardcoded RGB values;
  switching themes restyles them like every other surface.
- Streaming markdown keeps incremental layout after a `[label]`-style block:
  only the block a later reference definition could still relink stays
  re-rendered; earlier blocks keep committing instead of freezing layout for
  the rest of the message.
- The watch flood cap now guards the session, not each watch: 20 events a
  minute is one budget every watch draws on, so eight live watches can no
  longer pour 160 events a minute into the transcript and the model's context.
  The watch whose event crosses the budget still stops itself and says so.
- Background work no longer leaks for the life of the process: a finished
  watch keeps its event log only until eight later watches finish after it
  (older ones keep just their final event), and the controller's job-progress
  dedupe map drops a child slot's key as soon as the slot reports a terminal
  status instead of holding every sub-agent's keys forever.
- `edit` no longer clobbers a file that changed behind its back: the rewrite
  is staged in a temporary file and the file TAG is re-verified immediately
  before the swap, so a concurrent writer (sub-agent, watch command, the
  user's editor) fails the edit instead of being silently overwritten — and a
  crash mid-write can no longer truncate the file. An edit also preserves the
  file's existing permissions instead of resetting them.
- Every harness-owned file now swaps in through one atomic write path
  (staging file + fsync + rename). MEMORY.md joins config.yaml, UI preferences
  and usage history on it, so a crash or a concurrent Claude Code write can no
  longer leave a torn memory catalog both agents then read.
- Compaction summaries keep their read/modified file lists across restarts: the
  carry-forward was dead (mismatched detail types, a gate nothing ever set), so
  the second compaction of a session forgot every file the first one touched.
- LLM client hardening: error bodies are read with a size cap (a hostile
  endpoint can no longer OOM the harness with an oversized error page), the
  Responses client retries transient failures like the other protocols,
  stream-controlled tool-call indices are clamped, and usage history prunes
  entries unused for six months instead of growing forever.
- The model now sees only the tools the current plan state permits: while the
  plan is unapproved the provider receives just the exempt tools, and an
  approved plan shows exactly the union its in_progress steps allow — schemas
  for tools a step forbids never enter the context (the executor gate stays
  behind it as enforcement).
- App quit is no longer held hostage by a wedged sub-agent: shutdown bounds the
  waits for the active model run and for sub-agents to a shared 3-second budget
  and abandons the stragglers instead of hanging forever.
- config.yaml has one write path: the settings pane, the "Allow All for Every
  Session" toggle, and the `cozyphi config` editor all commit through a single
  serialized atomic editor, so concurrent saves no longer silently revert each
  other — and a save from the config editor no longer drops the plan.defaults
  section the page knows nothing about. The allow-all toggle also handles
  inline `permissions: {…}` mappings now and fails closed on an unparseable
  config instead of rewriting it.
- HTTP MCP servers get the same id discipline: an SSE body is scanned for the
  response whose JSON-RPC id matches the request — a progress notification or
  a foreign-id response arriving first is skipped instead of being returned as
  the answer, `data:` lines without a space parse again, and a body that never
  answers the request fails closed so the next call re-initializes.
- stdio MCP servers recover after a timeout instead of serving wrong answers:
  responses are matched to the request by JSON-RPC id (a late answer to an
  abandoned call, or a server-to-client request, is skipped rather than
  returned as the result), and a timeout or cancellation closes the server
  connection so the next call respawns and re-handshakes cleanly — the old
  behavior left the session attached to a desynchronized pipe that fed every
  later call a foreign result until restart.
- Permission gate now knows the `question` and MCP meta-tools instead of
  treating them as unknown actions: `question` passes (it is itself the ask —
  no more approval overlay in front of a question), `mcp_list`/`mcp_inspect`
  pass as read-only introspection, and `mcp_call` asks naming the
  `server/tool` it hands control to. Malformed `grep`/`find`/`mcp_call`
  arguments now fail the permission check instead of silently judging as
  path `.`.
- Session files survive crashes: the full flush rewrites through
  temp-file+rename (a crash mid-flush no longer destroys the transcript),
  a torn final line from a crashed append is dropped and trimmed on load
  instead of making the session unresumable, and a failed append rolls back
  in memory and trims its partial line on disk. A newline-terminated bad
  line still fails the load — corruption is not mistaken for a crash.
- Engine reconfiguration is race-free: mode, permission, hooks and model
  setters swap the client/executor pair under a lock while a running tool
  round works off an immutable snapshot, so changes land at round
  boundaries and a round always finishes under the posture it started with
  (previously a mid-run `/mode` toggle could race the streaming loop).
- Controller cleanup: the permission/continue/question asks share one
  generic publish-reply-timeout flow, `/resume` and `/clear` share one
  session-switch sequence, and the transcript replay projection moved to
  `internal/tui/transcript` beside the Mapper.
- New harness settings modal: `/settings`, the command palette, or `Ctrl+,`
  opens a full-screen editor for plan-gate policy stored in
  `~/.cozyphi/config.yaml` under `plan.defaults`. The **Plan defaults** tab
  edits the ordered step types, each type's allowed tools (with cascade to
  more/less capable types), and `Allowed outside plan` exemptions; the
  **General** tab shows the config path, scope, and live-apply status. Each
  tab scrolls independently; `Ctrl+S` validates and atomically saves the
  whole draft (owner-only file, unrelated YAML sections preserved,
  same-section external edits fail closed), `Esc` discards. Applied settings
  publish a live `plangate` policy that takes effect on the next inference —
  prompt, `plan` tool schema, gate checks, and plan validation all follow it
  without a restart, including after `/clear` or `/resume`.
- Step types are now fully configurable instead of a fixed set: rename a type
  and the current plan's references migrate atomically (approval preserved);
  deleting a type still used by the current plan is blocked; zero configured
  types blocks plan creation rather than falling back to defaults. The `plan`
  tool schema requires `type` and lists the configured types as its enum.
- `plan` is now steps-only: the model sends `{"steps":[...]}` and the harness
  atomically replaces the current plan under one lock, owning the revision.
  `plan action=get` is removed — the authoritative <current-plan> snapshot is
  injected into every inference as a transient assistant tool call and tool
  result, so the model sees harness data rather than a synthetic user request.
  With auto-approval enabled, a revised active plan is approved synchronously
  and the same tool result reports `approved:true`. Legacy
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

- Plans gain harness-run actions and per-step models. A plan action is a
  built-in command (`compact`, `inject_skill`) the harness fires at step
  boundaries (`step_start`/`step_end`) and at approval — including the
  auto-approval policy; the durable schema, the plan tool surface and the
  agent engine all carry them, unknown model or skill names reject the write
  at the tool seam with the valid options, run history stays a bounded tail
  that never re-enters through authoring, and failed runs land as a
  `⚙ plan` transcript row. Steps and step types can pin a model (step
  override beats the type default beats the session model): the engine
  resolves the pin at step start, refuses a transition naming an unknown
  model before any durable write, restores the session model when the plan
  closes, and a manual model switch supersedes the plan default. The sidebar
  plan pane shows action chips with run state and a per-step model picker
  with a "step type default" clear entry; the plan editor edits step
  automation rows (event, type, skills, remove, add) and model pins for
  steps and types, compiling them into `update_step`/`set_plan_fields`
  patches.
- Plan UX follow-ups: the sidebar plan pane names its keys in a standing
  hint row and takes focus with Alt+P (arrows select the step, `m` opens the
  model picker, Esc leaves; typing anything else hands keys back to the
  composer), and a step line badges the model it would run on (its own pin,
  else its type's). The plan editor's action event/type rows open choice
  screens with the current value preselected instead of cycling. /settings'
  plan tab gains a per-type Model row (inline picker, persisted with the
  other plan defaults) that new plans inherit as their type model map when
  their author pins none.
- Paste an image from the system clipboard into the composer: it is attached to
  the prompt (shown in the hints row), and sent to the model as an inline image
  content part across Anthropic, OpenAI chat, and OpenAI-responses protocols.
  Alt+X removes the attached image before submitting.
- Swapping two success criteria in the plan editor no longer writes a stray "!"
  into the plan on the way: the editor now frees the name it needs by removing
  the entry and putting it back, so every operation it saves carries a value you
  actually typed. Swapping them while the agent edits the same plan also merges
  instead of being reported as two conflicts and undone.
- The plan editor can add the first step to a plan that has none. "Add step"
  used to be refused outright, because inserting a step demanded an existing
  step to place the new one next to — and an empty plan has none. A whole plan
  authored in one pass, up to the 32-step cap, still saves as one atomic patch.
- The plan editor's controls now match the rest of the app. `Shift+↑↓` moves
  the selected step up or down the plan — on the step list, where the move is
  visible, and inside the step's details, where the title tracks it — replacing
  the "Move step up/down" rows that reordered a list you could not see. The
  editor speaks the context browser's motions (`j/k`, `gg`/`G`, `Ctrl+U/D`);
  every choice list opens with the cursor on the current value and is listed in
  `/help` with a footer drawn from the key catalog; and a delete confirmation
  names what it deletes — the step id, or the quoted criterion — instead of
  asking about "this step". In the context browser, `Backspace` no longer
  deletes entries behind the footer's back: it says which keys delete, and
  `/help` now lists `d` alongside `Del`.

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

[Unreleased]: https://github.com/alvnukov/cozyphi/compare/v0.19.0...HEAD
[0.19.0]: https://github.com/alvnukov/cozyphi/releases/tag/v0.19.0
[0.18.0]: https://github.com/alvnukov/cozyphi/releases/tag/v0.18.0
[0.17.0]: https://github.com/alvnukov/cozyphi/releases/tag/v0.17.0
[0.16.0]: https://github.com/alvnukov/cozyphi/releases/tag/v0.16.0
[0.15.0]: https://github.com/alvnukov/cozyphi/releases/tag/v0.15.0
[0.14.0]: https://github.com/alvnukov/cozyphi/releases/tag/v0.14.0
[0.13.0]: https://github.com/alvnukov/cozyphi/releases/tag/v0.13.0
[0.12.0]: https://github.com/alvnukov/cozyphi/releases/tag/v0.12.0
[0.11.0]: https://github.com/alvnukov/cozyphi/releases/tag/v0.11.0

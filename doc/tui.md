# TUI architecture

CozyPhi retains a complete `sessions.View` for every open session. The thin `editor.Editor` shell selects Views and drains all their buses through one App scheduler. Agent lifecycle lives in `internal/tui/controller`; session-to-widget projection lives in `internal/tui/transcript`.

`/new` opens a View; `/clear` replaces only the current conversation. `/switch N`, Ctrl+F10 (next), Shift+F10 (previous), and Alt+F10 (back) select retained state. Root event capture runs before focused widgets, so navigation remains available inside modals. The registry has 12 slots with stable live IDs independent of history IDs.
Sub-agents never take one of those slots: see [Interactive child assignments](#interactive-child-assignments).

Inactive Views keep drafts, widgets, asks and updates but cannot take focus or install a global editing profile. Each owns its history cursor, branch watcher and local shell. One microphone gate prevents overlapping capture; recording and delayed transcription remain with their originating View. Closing cancels owned UI work and waits for shell cleanup; a timeout is not proof of tool exit.

The clickable top selector uses `●` only for selection and `○` for other Views.
Separate labels show running, waiting, interrupted, stopped, error, unread turns
and live jobs. On narrow screens the selected session and previous/next arrows
remain available; keyboard navigation works through overlays. The composer names
its destination. Background attention appears as an origin-labelled clickable
notice with `/switch N` and the actual next-session key, never an implicit switch.
Desktop notifications retain the configured off/always/unfocused policy and name
the originating session. Unread counts completed turns, not permission requests;
it clears when the selected transcript is rendered at the bottom. This is a
bounded child selector, not the planned grouped multi-project sidebar.

## Interactive child assignments

Terminal `agent_spawn` creates a retained child View that never becomes a tab.
Each parent View owns a family of its children and draws one agent panel for them,
between the composer and the footer, on the parent screen and on every child screen
alike. Its first row is `main`, the session that owns the family; below it comes one
row per running child, plus a failed or stopped child inside its 30-second window.
`●` marks the row of the screen being drawn. Rows are titled `role(description)` —
the way the parent's transcript names the same child — and carry the tool count and
elapsed time the parent's sub-agent store already keeps; the panel invents nothing of
its own. A success leaves the panel at once, and the footer says `/agents to see
agents` for 30 seconds instead.

`↓` in the composer moves the keyboard into the panel once the caret sits at the end
of the last visual line and the history has no later entry to recall; `↑` on the
`main` row and `Esc` give it back. Inside a child's composer `Esc` keeps its ordinary
interrupt meaning, and Ctrl+C is claimed by the application before the panel ever
sees it. In the panel, `↑↓`/`j`/`k` select and `Enter` (or a click) opens: a child row
draws that child's session as the current screen, with no selector tab and no change
of selection, and the `main` row puts the parent back. `x` stops a running child
through the job-manager path `agent_cancel` uses, and clears a failed or stopped row
before its window runs out. `/close` on a child screen returns to the parent instead
of closing anything, and the attention notice with `/switch N` names only sessions
that have a tab. A parent retains at most 12 children of its own: when it is full the
oldest finished child is released first, a parent whose children are all running
refuses a new one, and `job.Manager.MaxConcurrent` still bounds running ones.

`/agents` (also in the palette) opens the full-screen agent browser over the current
session. It lists every child this session ever spawned, not only the ones the panel
still shows: the running ones on top in the order they were created, the finished ones
below with the newest first. A row carries the status glyph (`⟳` running, `⏸` waiting
on the child, `✓` done, `✗` failed, `■` stopped), the `role(description)` title, the
tool count and elapsed time when the job recorded them, and the outcome summary or the
error on the same line — a fact the job never recorded is left out rather than guessed.
It moves like every other list, `Enter` opens a child the parent still retains as the
current screen, `Enter` on a released child names the file its result was written to,
`x` stops a running child after a `y`/`n`, and `Esc` or `q` closes the browser. The
footer counts live children as `N agents`.

The first inference waits until the child View is fully assembled. A child uses the
ordinary Controller input queue; there is no second job queue or scheduler. Worker permission, question and continue
requests belong to that child. Opening it never approves a request. Explore/review
remain read-only, configured denials remain enforced, and children cannot acquire
nested agents, memory, tasks or watches through a mode or allow-all change.

A child asks wherever the user is. While a child has no screen of its own — the
parent's or a sibling's is up — its permission, continue and question panels open on
the screen the family is showing, headed `[role(description)]` so it is plain whose
call is being answered; on its own screen a child keeps its asks, unlabelled. The
panel holds one question at a time, so an ask that arrives while the user is
answering another one stays with the child, whose row reads `⏸ waiting: permission`
until the panel is free. The answer goes to that child alone: approve lets the one
call run, `Esc` denies the one call and the assignment carries on, and *Allow All for
This Session* binds to the child's own controller inside its role ceiling — the
parent keeps asking. *Allow All for Every Session* writes a rule that outlives every
session, so it is not offered on a child's ask at all. An ask stays on the screen it
was asked on: opening or leaving the child neither moves nor copies it, and a child
released with a question still open has it denied. The attention mark lands on the
parent, because a child has no tab for `/switch N` to name, and a child finishing
raises no desktop notification of its own — the parent's own turn end keeps that.

Interrupt cancels the current turn, not the assignment. Input accepted while work
is running continues at the existing input boundary. Leaving an interrupted child
without a queued continuation stops the assignment; leaving running work keeps it
running. In-session modals are not a leave. Stop retains partial assistant output
and a reason. Completed Views retain history, but release job admission capacity
only after the runner exits. New input to a terminal child admits a new linked
assignment; the previous result is immutable. Admission/binding failures are visible
and do not strand the retained child in an idle reservation.

A terminal outcome persists its owner, parent conversation, child session, job and
event identities, linkage, status, stop reason, human-intervention flag, and a
summary bounded to 12,000 bytes. Parent context and its delivery receipt are written
before source acknowledgement. Results are untrusted child data, not user commands
or approvals. Explicit `agent_wait` shares the receipt identity with automatic
delivery, preventing a second autonomous wake for an already consumed result.

The parent's spawn row stands for the child, not for the call that made it: it is
titled `role(description)` — every role, explore included — carries the skills
decision and a pinned model when there is one, and counts the child's tool rows
and elapsed time while it works. Interactive children report the same progress
headless ones do, for the lifetime of one assignment and no longer, so a retained
child between assignments is silent; those rows stay in the parent's transcript
and never enter the parent's context. A delivered outcome settles that same row —
summary, terminal glyph, stopped clock — instead of opening a second one. After a
resume, where the spawn row is projected away, the delivery receipt renders as a
local sub-agent row named after the child, or by its job id when the spawn call is
no longer in the history; it never reads as a user message.

Parents consume bounded batches at inference boundaries or use the existing watch
wake timer while idle. Explicit interrupt suppresses autonomous wake without losing
the result; the next user input can consume it. Clear/resume conversation fences
prevent late results entering another conversation. No cross-crash exactly-once
promise is made. Failed final writes retain the existing admission slot, bound
in-memory fallback by job capacity, and report an undelivered error; queries and
shutdown retry persistence. Unwritable storage is not crash-durable. Runtime and
headless shutdown surface outstanding outcome errors. Headless children remain
unattended and keep explicit spawn/wait/cancel behavior.

## Status dashboard

`/status` presents Status, Config, Usage and Stats in one modal pane. Each View
owns preference persistence and asynchronous history loading; the widget renders
safe snapshots. Config is read-only: the editor selects detached allowlisted
settings rows from its Store at open, without opening or forwarding events to
`settings.Pane`. Stats draws positioned colored calendar cells and responsive
metric columns rather than wrapping a textual heatmap. See
[status-dashboard.md](status-dashboard.md) for keys, data scope and lifecycle.

## Object aggregation

```
cmd/main.go
  ├─ editor.NewEditor(app, registry)   selection and shared redraw
  └─ sessions.NewView(app, bus, ctrl, …) for each retained session
       ├─ TranscriptPane   snap, list, mapper, subagents, welcome, text selection
       ├─ ComposerPane     chat, @/slash pickers (files + agent roles), palette,
       │                   Tab build/plan toggle (input only)
       ├─ FooterChrome     activity, spinner, tokens, update hint, hook status, watch indicator
       ├─ Sidebar          resizable runtime state + persistent session plan
       ├─ Overlays         permission ask, continue ask
       └─ Submitter        submit / cancel / slash / bash → Controller
```

### Aggregation rules

| Owner | Composes (lifecycle) | Aggregates (injected) |
| ----- | -------------------- | --------------------- |
| `Editor` | selection/event routing | `Registry`, `App` |
| `View` | all panes, `Submitter`, `toast`, UI lifetime | its `Bus`, `Controller`, shared `App`/`vx` |
| `TranscriptPane` | `MessageList`, `Mapper`, `SubagentStore`, `welcome`, `textSel` | `theme`, `spinner` ref from footer |
| `ComposerPane` | `ChatInput`, pickers, `palette` | callbacks `onSubmit`, `onCancel`, `onRedraw` |
| `FooterChrome` | `ActivityHandler`, `Spinner` | `labelContext()`, `liveJobs()` closures |
| `Overlays` | `permAskState`, `continueAskState` | `activity` ref, reply callbacks |
| `Submitter` | `BashRunner` | `Controller`, `Bus`, `CommandRegistry`, pane refs |

**Hard rule:** no `*Editor` back-pointers on handlers. Cross-domain work uses injected refs, callbacks, or `Bus.Publish`.

---

## Package layout

```text
internal/tui/
├── editor/                 # process shell, selection and event capture
├── sessions/               # full retained View, registry, layout, branch watch, command bridge
├── controller/             # Engine lifecycle, Bus/Msg, activity, permission replies
├── transcript/             # Mapper, SubagentStore, TranscriptPane
├── composer/               # ComposerPane, Wire(), Input iface
├── footer/                 # FooterChrome, composer usage labels, live-watch indicator
├── sidebar/                # fixed runtime state + independently scrolling plan
├── overlays/               # permission + continue ask
├── settings/               # harness settings modal (tabs, draft, apply/discard)
├── submit/                 # Submitter, BashRunner
├── commands/               # registry, builtins, SessionCommands, HookCommands
├── tokens/                 # token formatting + context-fill tiers (footer, sidebar)
└── pathutil/               # short path + git branch labels
```

| Package | Role |
| ------- | ---- |
| `editor` | TUI root; drains all Views and renders only the selected one |
| `sessions` | Retained widget graphs, activation/focus, UI lifetime and registry |
| `controller` | `Controller` runs `agent.Engine`; publishes `Msg` to the bus only |
| `transcript` | Projects `session.Event` → message list; sub-agent rows; turn metadata row; copy selection |
| `composer` | Keyboard routing for chat, `/` slash, `@` mention, Ctrl+K palette, Tab mode |
| `footer` | Spinner, activity line, token/context labels, update hint, hook status, live-watch indicator (a breathing `⏱`, count, labels; a click folds/unfolds the watch's transcript rows, `WatchesAt` maps the column back to the watch); the row is clipped with an ellipsis, never under the hint |
| `watchpane` | Full-screen watch browser (`/watches`, `Ctrl+W`): list with state/age, log popup, stop-with-confirm — a dumb view over the controller's watch seams |
| `agentlist` | Full-screen sub-agent browser (`/agents`): this session's children with status, tools, elapsed and outcome; open, stop-with-confirm — a dumb view over the family's seams |
| `sidebar` | Resizable right panel (Ctrl+O): fixed runtime/context/subscription/MCP state above a separately scrolling durable plan. Visibility never controls model access to the plan |
| `overlays` | Modal permission / continue-ask panels; replaces composer when active |
| `settings` | Full-screen harness settings modal (`/settings`, palette, `Ctrl+,`); tabs `Plan defaults` + `General`, per-tab scroll, whole-draft `Apply` via `harnesssettings.Manager`; dumb view over `settings.Store` |
| `submit` | User submit path: agent prompt, slash commands, `!bash`, cancel |
| `commands` | Slash/palette registry; session load/clear; hook command bridge. Slash args parse via `DispatchSlash`; commands may carry an `ArgCompleter` the `/` picker offers in the first argument (`/theme`, `/model`) |
| `tokens` | Token count formatting and context-fill tiers shared by usage displays |
| `pathutil` | Cwd shortening and git branch labels for composer chrome |

Dumb rendering widgets stay in `internal/components/` (chat, input, palette, mention, transcript blocks, …).

All color knowledge lives in `components.Theme`. Besides the chrome roles, it carries
two role groups ported verbatim from opencode's `opencode.json` (`theme.markdown*` /
`theme.syntax*`): `Markdown` (heading, strong, emph, inline code, link labels, quote,
list markers, plain code) and `Syntax` (comment, keyword, function, variable, string,
number, type, operator, punctuation). Renderers consume roles — they never improvise
by reusing `Warning`/`Success` as code or heading colors; that improvisation is what
broke palette parity once. Bundled legacy themes (Dark, Darcula, Pink, Terminal) fill
the same groups via `legacyMarkdownAndSyntax` to keep their old look; paths in prose
keep the base color and only gain an underline.

Message layout follows opencode's session route too. The transcript list insets
entries two columns per side; user prompts render as panels (a `Secondary`
full-height ┃ rule, `BackgroundPanel` fill, one blank panel row above and below
the text, text inset two columns); assistant-side blocks (assistant text,
thinking, tools, bash, sub-agents) open three columns in
(`block.messageIndent`) and expand their bodies two more; the end-of-turn
footer reads `▣ model[ctx] · duration` — marker in `Secondary`, model label in
`Foreground`, remainder muted; compaction draws a centered ` Compaction ` rule
in the border color. Legacy themes keep their chrome via `legacyChrome`
(Secondary = Accent, panel = default background).

The composer mirrors opencode's prompt: a left ┃ bar in the posture color
(build `Secondary`, plan `Warning`, `!` shell prefix `ToolName`) wraps a
`BackgroundElement` panel with the `⏵⏵ posture · model` meta row inside its
bottom edge, a `╹▀` tail fades the frame out, and the row below carries the
cwd (muted) and usage spans. An empty input shows the muted placeholder —
`Ask anything...`, swapped for `Run a command...` while the shell prefix is
active. The composer's smallest height is `ChatInput.MinHeight` — the pane
and the editor layout clamp against that one number instead of re-deriving
the floor.

Pasting an image from the system clipboard attaches it to the prompt instead
of inserting text: the hints row shows `📷 image/png`, and on Enter the image is
sent to the model as an inline image content part (Anthropic, OpenAI chat,
OpenAI-responses). `Alt+X` removes the attached image. A clipboard that holds
text still pastes text normally.

---

## Assembly (`cmd/main.go`)

`cmd` owns project/config loading and constructs collaborators **before** the TUI root:

```text
cmd/main.go
  process := controller.NewRuntime(project, usageHistory)
  process.EnableInteractiveChildren() // before binding the first Engine runner
  workspace := process.Workspace(cwd)
  settingsManager := harnesssettings.Open(..., process.PlanRuntime(), nil)
  registry := sessions.NewRegistry(12, application.RequestRedraw)
  ui := editor.NewEditor(application, registry)
  redraw.Bind(ui.RequestRedraw)

  each main/child View:
    bus / Controller bound to its own workspace and acquired history owner
    newTUIView(..., shared history corpus, settingsManager, captureGate)
    registry.Open(name, view)

  ui.SetSessionSync(...) // attaches child Views before releasing first inference
  application.Run(ui)
```

Inside `sessions.NewView`, panes are built in dependency order:

1. `FooterChrome` and `Sidebar` — spinner, activity, right runtime/plan panel (need `contextWindow`)
2. `TranscriptPane` — shares footer spinner; usage callback → footer label + sidebar turns
3. `ComposerPane` — chat chrome; footer binds composer for labels
4. `Overlays` — permission/continue UI; uses footer activity + composer focus
5. `SessionCommands`, `HookCommands`, `BashRunner`, `Submitter` — explicit deps, no `*Editor` fields
6. `settings.Pane` — optional `settings.Store` (variadic); current-plan type usage and runtime tool availability are pushed in via callbacks
7. `ComposerPane.Wire(...)` — connects composer keyboard path to submitter, overlays, bus

`Editor` does **not** call `project.GetDefaultProject` or construct `Controller`.

### Runtime, workspace and session ownership

`controller.Runtime` owns the process catalog, shared job manager and immutable
plan defaults. `Runtime.Workspace(cwd)` canonicalizes the directory and retains
hooks, MCP, LSP, project configuration and corpus references for that workspace.
Distinct worktrees do not collapse to the common Git root. MCP stdio processes
start in that explicit canonical directory, not the process's ambient cwd.

`Runtime.NewSession` constructs an independent Controller/Engine/Bus relationship.
Turns, input queues, watches, permissions, mutable plans, approvals and edit
capabilities are session-local. `history.Store.NewCursor` shares persistence and
entries while retaining independent navigation and recalled drafts. The legacy
`NewController` wrapper owns a private Runtime for single-session callers.

A fresh live owner ID scopes child jobs independently of the persisted session
ID: Clear/Resume cannot orphan running children, and reopening history does not
reuse an old owner's shutdown tombstone. Agent tools and progress routing check
that owner; session IDs still correlate history. Runners capture the engine's
model, role pins, hooks and LSP at tool binding rather than borrowing a sibling's
settings at execution time.

Closing a Controller cancels only its own jobs and turn. Runtime shutdown closes
admission and cancels active sessions before waiting for constructors or runners.
Startup hooks run outside the admission lock and receive shutdown cancellation.
The caller's bounded wait is not proof that tools stopped: actual cleanup retains
shared services until constructors, sessions, jobs and final writes have exited.
Workspaces are retained until process shutdown, not the last session's Close.

Headless `run` keeps its direct Engine loop and private job manager, with explicit
canonical workspace assembly and the same snapshot binding. It gains no UI
scheduler or implicit wake loop; explicit waits and exit-time reaping remain.

### First run without a model

`LoadConfig` plants a fully commented `~/.cozyphi/config.yaml` when the file
is missing (owner-only, never rewritten, a write failure is only a warning) and
treats zero models or a missing `api_key` as warnings — so the TUI always
starts. `NewController` resolves the startup model in this order: `COZYPHI_MODEL`,
the config default, the last model the user picked (`ui.json`), then the first
model of the runtime catalog (connected providers, then opencode). An
automatic catalog pick is named in a startup toast and never recorded as the
user's last model.

With no model anywhere, the footer and composer show a `no model` placeholder
(`EffectiveModelName`) and `StartPrompt` refuses the submit — an error row with
the same notice plus `RunEndedMsg`, no turn, no request. Headless `cozyphi run`
has no screen to refuse on, so it exits before connecting with the three ways
to configure a model: edit the config file (`cozyphi config`), `/connect` in
the TUI, or the `COZYPHI_*` environment.

---

## Main-screen model and effort controls

Click the model name below the input to choose a model, then its reasoning effort.
Click the separate effort field to change only the current model's effort.
Both fields and selectable menu rows highlight under the pointer; menu rows accept
a single click, and the wheel scrolls the choices. The draft and caret survive a
choice. Models without selectable effort levels show only the model control.

**F5** opens the current model's effort picker in standard, Bash/Readline and Vim
input styles. Rebind it with `keybinds: {effort: "F9"}`; F1 help follows that binding.
The `default` choice restores the model/provider default. A validated model/effort
pair applies atomically to the next inference, including while a turn is running.
The current inference and its tool round retain their original snapshot. Invalid
effort leaves the selection unchanged. The composer distinguishes a pending choice
from the running pair, and shows the user's choice when a plan pin takes precedence.
Switching changes neither gates nor role ceilings; children retain their profile
and never overwrite the main session's last-used model preference.

## Input styles

Use `/keymap` (also available in the command palette) to choose `standard`,
`readline` or `vim`. The default is standard; switching preserves the draft and
caret and saves only the preference in global `ui.json`. Invalid choices or
conflicting custom bindings leave both the live profile and saved preference
unchanged. `bash` and `emacs` are aliases for readline.

**F6** cycles `standard → readline → vim → standard` through the same saved
preference, preserving the draft and caret; entering Vim starts in INSERT.
Rebind with `keybinds: {keymap: "F9"}` or disable with `keymap: "none"`.
F1 help follows the binding. Modal screens keep ownership of their keys.

The composer reserves space for its editing-mode badge even when the model
label is long. Hints adapt to terminal width and cannot overwrite the path.
`F1` / `/help` reloads the current binding catalog on every open.

| Profile | Editing |
| --- | --- |
| Standard | Existing selection, clipboard, arrows and history behavior. `Ctrl+Z` undoes; `Ctrl+Y` or `Ctrl+Shift+Z` redoes. |
| Readline | `Ctrl+A/E` logical line start/end, `Ctrl+B/F` character, `Alt+B/F` word, `Ctrl+P/N` line/history, `Ctrl+H/D` backward/forward delete. `Ctrl+U/K/W` kill before caret/to line end/previous whitespace word, `Alt+D` kills forward, `Ctrl+Y` restores the last kill. `Cmd+A` still selects all. |
| Vim | Starts in INSERT. `Esc` enters NORMAL; `i/a/I/A` resume insertion, `o/O` open a line. NORMAL supports `h/j/k/l`, `w/b`, `0/^/$`, `gg/G`; `x/dd/dw/D` delete, `cc/cw/C` change, `yy/yw/y$` yank and `p` puts. `u` / `Ctrl+R` undo/redo. |

Readline frees its editing chords by moving palette, plan editor, watches,
transcript expansion, plan approval and plan details to `F2/F3/F4/Shift+F6/F7/F8`
respectively. User `keybinds` overrides are retained if they do not conflict.
Help and palette labels follow the same table used by dispatch.

Vim is a composer dialect, not a complete Vim implementation: counts, visual
mode, macros and ex commands are not implemented. Enter sends only in INSERT;
NORMAL consumes bare Enter. A picker or voice dialog closes before Esc changes
editing mode. With Vim input focused, use `Ctrl+C` to interrupt work; Esc is
reserved for the editor. Pasted text enters INSERT and is never executed as Vim
commands. Slash commands are typed in INSERT. NORMAL uses the shared hotkey
layout mapping (including Russian ЙЦУКЕН and terminal-reported alternate keys),
with uppercase/Shift commands preserved. INSERT always keeps the original text.

Undo records each input event (including selection replacement, completion or a
paste) as one change; Vim groups the entire INSERT session, including the
command that started it. History is bounded to 100 revisions and 1 MiB of text
per direction. A new edit
drops redo, and an externally replaced draft clears incompatible history.
Undo storage and the last killed/yanked text are session-local.

## UI goroutine loop

```text
xui event
  └─ Editor.Handle → settings modal (when visible, consumes everything)
       → ComposerPane.Handle (keys, paste, focus)
       ├─ overlay keys → Overlays (when active)
       ├─ copy keys    → TranscriptPane
       └─ submit       → bus.Publish(SubmitMsg)

app frame
  └─ Editor.Draw
       ├─ drainBus()          # apply pending Msg batch on UI thread
       ├─ layout: list | chat/overlay | footer (+ right sidebar, Ctrl+O)
       └─ toast overlay (if visible)
```

`RequestRedraw` schedules a background frame. The bus coalesces high-frequency
stream events; one armed wake can cover many publishes until the next `Drain`.
Scheduled stream/animation frames are capped at 20 fps so Markdown layout
cannot monopolize the UI goroutine. Keyboard events redraw directly and are not
delayed by that cap.

The transcript owns a mutable session reducer on the UI goroutine. A streaming
update whose tail row IDs are unchanged projects and patches only the last
message; replay, cancellation, cross-message thinking coalescing, and any row
shape change fail closed to the full `Mapper.Sync` path. Historical rows are
therefore not copied, projected, indexed, or patched for every token.

Assistant Markdown follows the same stable-prefix rule within the active row:
completed top-level blocks keep their parsed lines, only the final block is
reparsed as text is appended, and only changed visual rows are repainted into a
persistent surface. Prefix edits, width/theme changes, reference-link syntax,
and unsupported block shapes reset to the exact full renderer. Selection and
block highlighting clone cached surfaces before styling them.

A lone Esc byte is held by the input parser (it might start a sequence); the xui read loop flushes it as `KeyEscape` once input stays quiet for 50 ms (`Parser.Pending`/`FlushIdle`), so every Esc handler — permission overlay, palette, slash menu — actually fires.

---

## Bus: publish and drain

**Publish** (any goroutine): widgets, `Controller`, background tasks (`StartBranchWatch`, `StartUpdateCheck`), hook commands.

**Drain** (UI goroutine only, at start of `Draw`):

| Phase | Messages | Handler |
| ----- | -------- | ------- |
| Batch pass | `SessionEventMsg`, `JobProgressMsg` | `TranscriptPane` → optional `Sync` + footer token refresh |
| Per-msg | everything else | `Editor.Update` → domain handler |

### Message routing

| `controller.Msg` | Handler |
| ---------------- | ------- |
| `SessionEventMsg`, `JobProgressMsg` | `TranscriptPane` (in `drainBus`) |
| `PlanUpdatedMsg` | `Sidebar.SetPlan` after the session append succeeds |
| `SubmitMsg`, `CancelStreamMsg` | `Submitter` |
| `PermissionAskMsg`, `PermissionDismissMsg`, `ContinueAskMsg`, `ContinueDismissMsg` | `Overlays` |
| `SetActivityMsg`, `ClearIfActivityMsg`, `RunEndedMsg`, `UpdateAvailableMsg`, `HookSessionEffectsMsg` | `FooterChrome` |
| `MentionResultsMsg`, `BranchLabelMsg` | `ComposerPane` |
| `VoiceStateMsg` | `ComposerPane` hint row + `FooterChrome` activity (`Editor.applyVoiceState`): `ActivityListening`, `ActivityVoicePaused`, `ActivityTranscribing` |
| `VoiceResultMsg` | `ComposerPane` (insert one segment at the caret); the mode stays on, so the footer is left alone |
| `VoiceErrorMsg` | `ComposerPane` (cancels a pending send) + error toast; the mode stays on |
| `VoiceNoticeMsg` | warning toast (the silence auto-pause and other non-fatal notices) |
| `HookCommandResultMsg` | `HookCommands` |
| `RedrawMsg` | no-op (redraw already scheduled) |

---

## Interaction flows

### 1. Agent submit

```text
User Enter in composer
  → ComposerPane publishes SubmitMsg{text}
  → drainBus → Submitter.Submit
       ├─ "!cmd" prefix  → BashRunner (local shell, SessionEventMsg for output)
       ├─ "/slash"       → CommandRegistry / SessionCommands / HookCommands
       └─ plain text     → Controller.StartPrompt
                              ├─ no model → refused: error row with the setup hint + RunEndedMsg, no turn
                              ├─ idle   → agent.Engine.Loop (background)
                              └─ active → FIFO queue → Engine.Loop after current exit
                                             └─ SessionEventMsg, SetActivityMsg, PermissionAskMsg, …
```

`Submitter` clears accepted composer input and snapshots pending skills. A prompt
submitted while a local `!cmd` is running stays in the composer and produces an
explicit warning; model runs queue prompts instead of rejecting them — the one
non-queued refusal is a session with no model at all (see *First run without a
model*).

### 2. Stream and transcript

```text
Controller.runLoop
  → engine.Loop events
  → bus.Publish(SessionEventMsg{Event})
  → drainBus: TranscriptPane.ApplySession
  → TranscriptPane.Sync (tail patch, or full mapper fallback)
  → stick-to-bottom if user was pinned
```

`JobProgressMsg` updates nested sub-agent tool rows without full thread resync when the tree is unchanged.

The primary engine exposes one action-based `plan` tool: `get` returns the
canonical snapshot and `update` replaces the whole list using its revision.
Validation allows at most one `in_progress` item; the session manager appends
the complete snapshot as `EntryPlan`, without moving the conversational leaf or
putting it in provider context. Only after that append succeeds does `PlanUpdatedMsg` reach
the UI. Resume restores the latest snapshot immediately. Ctrl+O changes only
sidebar visibility, and mouse-wheel events over the fixed status area are
consumed without scrolling the transcript; wheel events over the plan move its
own viewport.
Ctrl+Up/Down scrolls that viewport by one row and Ctrl+PageUp/PageDown by one
page; these bindings are inactive while the panel is hidden. A new revision
automatically reveals its `in_progress` step when it fits in the viewport.
Ctrl+D flips the plan pane between the brief view (goal, progress, active
step, blockers) and the expanded rationale view (approach, working context,
per-step why/done_when/outcome/evidence refs, blocked resume condition); both
views share the one viewport, so the scroll position survives the flip. When
a material revision revokes approval, the pane opens with the bounded diff
against the last approved snapshot — `reapproval: N changes` plus
`target.field` lines — never the replaced prose; a finished plan shows a
`closed: <result>` row that a reopen removes.

### 3. Cancel

```text
Esc / composer cancel
  → CancelStreamMsg
  → Submitter.Cancel → Controller cancels the current stream context
                         └─ accepted queued prompts remain FIFO
  → ClearIfActivityMsg when activity was cancelled
```

### 4. Permission / continue ask

```text
Engine needs approval
  → Controller publishes PermissionAskMsg (or ContinueAskMsg)
  → Overlays.Apply → replaces composer bottom panel
  → user keys → Overlays → Controller reply channel
  → PermissionDismissMsg / ContinueDismissMsg
```

Composer input is blocked while an overlay is active (`OverlayBlocksComposer`).

### 5. Slash / palette / hooks

```text
/something or Ctrl+K
  → ComposerPane local UI OR SubmitMsg with slash text
  → Submitter.dispatchSlash → CommandRegistry
  → SessionCommands (/clear, /resume, …) or builtins
  → HookCommands (async) → HookCommandResultMsg → palette push / toast
```

`commandBridge` in `editor` builds `commands.CommandContext` for builtins (model switch, theme, permissions, copy last message, …).

Executed slash commands land in the prompt history (`prompt-history.jsonl`): a
picker-accepted no-argument command submits through `Chat.OnSubmit` — the one
submit path — and a typed command records on Enter. Up/Down recall at the row
edges walks the history; a walk started from a '/'-leading draft (picker closed
with Esc, or the caret past the command token) visits only slash entries.

The history is also searchable, bash-style. Ctrl+R (`keys.CmdHistorySearch`)
enters **reverse-i-search** in the composer: typing edits the query (matches
are case-insensitive substrings, newest first, `Store.Search`), each further
Ctrl+R steps older and Ctrl+S (`keys.CmdHistorySearchForward`, only while the
search is on) steps newer. The body previews the current match with the
query highlighted; the draft is untouched until a key ends the mode. Enter
submits the match through `Chat.OnSubmit`; Esc, Tab and the arrows accept it
into the buffer without sending; Ctrl+G aborts and restores the draft
exactly — the pane resolves the voice chord to the abort first while a search
is active, and to the voice toggle otherwise. The mode lives in
`internal/components/chat/search.go` behind `SearchActive`/`BeginSearch`/
`SearchOlder`/`SearchNewer`/`SearchAbort`; the pane only resolves chords, so
the widget package stays free of `internal/tui/keys`. Known deviation from
bash: after the arrows end the search, Up/Down walk the history from the
newest entry rather than from the match's position (the store walk is not
indexed by the match).

### 6. Voice input

Ctrl+G toggles **voice dialog mode**: the microphone stays open and each
utterance is transcribed as its own segment while the user keeps talking. See
`doc/voice.md` for the user-facing loop.

```text
Ctrl+G in composer (keys.CmdVoice)
  → ComposerPane.ToggleVoice → Editor (VoiceController) → voice.Session.Start/End
Space / Enter / Esc in composer (only while the mode is on)
  → ComposerPane.handleVoiceKey → VoicePause / VoiceResume / VoiceFlush / VoiceDiscard
       └─ session goroutine → Editor.publishVoiceEvent → Bus
              ├─ VoiceStateMsg     → hint row + ActivityListening / VoicePaused / Transcribing
              ├─ VoiceResultMsg    → ReplaceRange(caret, caret, text), one segment at a time
              ├─ VoiceErrorMsg     → toast; a pending send is cancelled
              └─ VoiceNoticeMsg    → toast
```

Space is a control key while the mode is on: a press flips the microphone at
once, a release flips it back when the key was held for at least 300 ms, so tap,
hold-to-pause and push-to-talk are one rule. Releases arrive only under the
kitty keyboard protocol (`vx.Caps().KittyKeyboard`, passed in as
`VoiceOptions.HoldKeys`); without them every press is a tap and the hint row
never promises holding. Space with a modifier, or with a picker open, reaches
the chat input unchanged.

Enter closes the open segment and waits for the queue to drain before
submitting (`⋯ finishing… then send`); Esc cancels that pending send first, and
ends the mode discarding everything only when nothing is pending. Results
carrying a stale `Gen` are dropped, the way `MentionResultsMsg` is.
`ActivityListening`, `ActivityVoicePaused` and `ActivityTranscribing` never
override a running stream activity — `ActivityHandler.Apply` refuses a voice
activity while a run holds the footer, and idle clears all three through
`ClearIfActivityMsg` so a run that started meanwhile keeps its own label.

### 7. Background chrome

| Source | Msg | Target |
| ------ | --- | ------ |
| `StartBranchWatch` | `BranchLabelMsg` | composer bottom-right label |
| `StartUpdateCheck` | `UpdateAvailableMsg` | footer update hint |
| Hook session lifecycle | `HookSessionEffectsMsg` | footer status + toast |

---

## Layering vs `internal/components`

| Layer | Responsibility |
| ----- | -------------- |
| `internal/components/*` | Draw/handle only; no bus, no engine |
| `internal/tui/*` | State, routing, session projection, submit |
| `internal/tui/controller` | Agent engine, jobs, permission gate, hooks/MCP |
| `cmd` | Config, xui, bus/controller construction, `NewEditor` |

Reference implementation patterns: panda `interactive.go` (assembly), `message.go` (transcript), `submit.go` (submit/cancel/bash).

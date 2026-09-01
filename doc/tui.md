# TUI architecture

CozyPhi’s interactive UI follows a **panda-style** split: a thin `Editor` root widget, domain handlers that **own their state**, and dumb widgets under `internal/components`. Agent lifecycle lives in `internal/tui/controller`; session→widget projection lives in `internal/tui/transcript`.

## Object aggregation

```
cmd/main.go
  └─ editor.NewEditor(app, bus, ctrl, …)
       ├─ TranscriptPane   snap, list, mapper, subagents, welcome, text selection
       ├─ ComposerPane     chat, @/slash pickers (files + agent roles), palette,
       │                   Tab build/plan toggle (input only)
       ├─ FooterChrome     activity, spinner, tokens, update hint, hook status
       ├─ Sidebar          resizable runtime state + persistent session plan
       ├─ Overlays         permission ask, continue ask
       └─ Submitter        submit / cancel / slash / bash → Controller
```

### Aggregation rules

| Owner | Composes (lifecycle) | Aggregates (injected) |
| ----- | -------------------- | --------------------- |
| `Editor` | all panes, `Submitter`, `toast` | `Bus`, `App`, `vx` |
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
├── editor/                 # Editor root: layout, dispatch, branch watch, command bridge
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
| `editor` | TUI root `components.Widget`; wires panes; `Draw` drains the bus |
| `controller` | `Controller` runs `agent.Engine`; publishes `Msg` to the bus only |
| `transcript` | Projects `session.Event` → message list; sub-agent rows; turn metadata row; copy selection |
| `composer` | Keyboard routing for chat, `/` slash, `@` mention, Ctrl+K palette, Tab mode |
| `footer` | Spinner, activity line, token/context labels, update hint, hook status, live-watch count/labels; long text cuts with an ellipsis (`layout.EllipsizeToWidth`), never under the hint |
| `watchpane` | Full-screen watch browser (`/watches`, `Ctrl+W`): list with state/age, log popup, stop-with-confirm — a dumb view over the controller's watch seams |
| `sidebar` | Resizable right panel (Ctrl+O): fixed runtime/context/MCP state above a separately scrolling durable plan. Visibility never controls model access to the plan |
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
proj.LoadConfig()
vx, theme, cwd
redraw := controller.NewRedrawRelay()
bus    := controller.NewBus(redraw.Fire)
ctrl   := controller.NewController(bus, proj, cwd)
cmds   := commands.NewBuiltinRegistry()
setts  := harnesssettings.Open(proj.Global().ConfigFile(), ctrl.PlanRuntime(), ctrl)
ui     := editor.NewEditor(app, bus, ctrl, cmds, vx, theme, cwd, model, skillPath, contextWindow, modelNames, setts)
redraw.Bind(ui.RequestRedraw)
ui.StartUpdateCheck(...)
ui.StartBranchWatch()
app.Run(ui)
```

Inside `NewEditor`, panes are built in dependency order:

1. `FooterChrome` and `Sidebar` — spinner, activity, right runtime/plan panel (need `contextWindow`)
2. `TranscriptPane` — shares footer spinner; usage callback → footer label + sidebar turns
3. `ComposerPane` — chat chrome; footer binds composer for labels
4. `Overlays` — permission/continue UI; uses footer activity + composer focus
5. `SessionCommands`, `HookCommands`, `BashRunner`, `Submitter` — explicit deps, no `*Editor` fields
6. `settings.Pane` — optional `settings.Store` (variadic); current-plan type usage and runtime tool availability are pushed in via callbacks
7. `ComposerPane.Wire(...)` — connects composer keyboard path to submitter, overlays, bus

`Editor` does **not** call `project.GetDefaultProject` or construct `Controller`.

---

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
                              ├─ idle   → agent.Engine.Loop (background)
                              └─ active → FIFO queue → Engine.Loop after current exit
                                             └─ SessionEventMsg, SetActivityMsg, PermissionAskMsg, …
```

`Submitter` clears accepted composer input and snapshots pending skills. A prompt
submitted while a local `!cmd` is running stays in the composer and produces an
explicit warning; model runs queue prompts instead of rejecting them.

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

### 6. Background chrome

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

# TUI architecture

Phi’s interactive UI follows a **panda-style** split: a thin `Editor` root widget, domain handlers that **own their state**, and dumb widgets under `internal/components`. Agent lifecycle lives in `internal/tui/controller`; session→widget projection lives in `internal/tui/transcript`.

## Object aggregation

```
cmd/main.go
  └─ editor.NewEditor(app, bus, ctrl, …)
       ├─ TranscriptPane   snap, list, mapper, subagents, welcome, text selection
       ├─ ComposerPane     chat, @/slash pickers (files + agent roles), palette,
       │                   Tab build/plan toggle (input only)
       ├─ FooterChrome     activity, spinner, tokens, update hint, hook status
       ├─ Sidebar          right status panel: context fill, turn tokens, MCP servers
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
├── footer/                 # FooterChrome, composer usage labels
├── sidebar/                # right status panel: context, tokens, MCP servers
├── overlays/               # permission + continue ask
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
| `footer` | Spinner, activity line, token/context labels, update hint, hook status |
| `sidebar` | Right status panel (Ctrl+O): context fill bar, recent turn tokens, MCP servers |
| `overlays` | Modal permission / continue-ask panels; replaces composer when active |
| `submit` | User submit path: agent prompt, slash commands, `!bash`, cancel |
| `commands` | Slash/palette registry; session load/clear; hook command bridge |
| `tokens` | Token count formatting and context-fill tiers shared by usage displays |
| `pathutil` | Cwd shortening and git branch labels for composer chrome |

Dumb rendering widgets stay in `internal/components/` (chat, input, palette, mention, transcript blocks, …).

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
ui     := editor.NewEditor(app, bus, ctrl, cmds, vx, theme, cwd, model, skillPath, contextWindow, modelNames)
redraw.Bind(ui.RequestRedraw)
ui.StartUpdateCheck(...)
ui.StartBranchWatch()
app.Run(ui)
```

Inside `NewEditor`, panes are built in dependency order:

1. `FooterChrome` and `Sidebar` — spinner, activity, right status panel (need `contextWindow`)
2. `TranscriptPane` — shares footer spinner; usage callback → footer label + sidebar turns
3. `ComposerPane` — chat chrome; footer binds composer for labels
4. `Overlays` — permission/continue UI; uses footer activity + composer focus
5. `SessionCommands`, `HookCommands`, `BashRunner`, `Submitter` — explicit deps, no `*Editor` fields
6. `ComposerPane.Wire(...)` — connects composer keyboard path to submitter, overlays, bus

`Editor` does **not** call `project.GetDefaultProject` or construct `Controller`.

---

## UI goroutine loop

```text
xui event
  └─ Editor.Handle → ComposerPane.Handle (keys, paste, focus)
       ├─ overlay keys → Overlays (when active)
       ├─ copy keys    → TranscriptPane
       └─ submit       → bus.Publish(SubmitMsg)

app frame
  └─ Editor.Draw
       ├─ drainBus()          # apply pending Msg batch on UI thread
       ├─ layout: list | chat/overlay | footer (+ right sidebar, Ctrl+O)
       └─ toast overlay (if visible)
```

`RequestRedraw` → `vx.QueueRefresh()`. The bus coalesces high-frequency stream events; one armed wake can cover many publishes until the next `Drain`.

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
| `SubmitMsg`, `CancelStreamMsg` | `Submitter` |
| `PermissionAskMsg`, `PermissionDismissMsg`, `ContinueAskMsg`, `ContinueDismissMsg` | `Overlays` |
| `SetActivityMsg`, `ClearIfActivityMsg`, `UpdateAvailableMsg`, `HookSessionEffectsMsg` | `FooterChrome` |
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
       └─ plain text     → Controller.Submit → agent.Engine.Loop (background)
                              └─ SessionEventMsg, SetActivityMsg, PermissionAskMsg, …
```

`Submitter` clears composer input after slash/bash; agent submit passes pending skills from composer.

### 2. Stream and transcript

```text
Controller.runLoop
  → engine.Loop events
  → bus.Publish(SessionEventMsg{Event})
  → drainBus: TranscriptPane.ApplySession
  → TranscriptPane.Sync (mapper + subagent store)
  → FooterChrome.SyncFromSnap (tokens / context window)
  → stick-to-bottom if user was pinned
```

`JobProgressMsg` updates nested sub-agent tool rows without full thread resync when the tree is unchanged.

### 3. Cancel

```text
Esc / composer cancel
  → CancelStreamMsg
  → Submitter.Cancel → Controller cancels stream context
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

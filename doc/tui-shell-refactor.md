# TUI shell refactor plan

Replace the monolithic `Editor` god object with a **panda-style** layout: thin `Shell` root widget, domain handlers that **own their state**, and dumb `internal/components` widgets. Agent lifecycle stays in `internal/tui/controller`; session→widget projection stays in `internal/tui/transcript`.

Reference: `panda/panda/interactive.go` (assembly), `panda/panda/message.go` (transcript handler), `panda/panda/submit.go` (submit/cancel/bash).

## Target architecture

```
cmd/main.go
  └─ NewShell(app, bus, ctrl, …)
       ├─ TranscriptPane   snap, list, mapper, subagents, welcome, text selection
       ├─ ComposerPane     chat, @/slash pickers, palette (input only)
       ├─ FooterChrome     activity, spinner, tokens, update hint, hook status
       ├─ Overlays         permission ask, continue ask
       └─ Submitter        submit / cancel / slash / bash → Controller
```

### Aggregation rules

| Owner | Composes (lifecycle) | Aggregates (injected) |
| ----- | -------------------- | --------------------- |
| `Shell` | all panes, `Submitter`, `toast` | `Bus`, `App`, `vx` |
| `TranscriptPane` | `MessageList`, `Mapper`, `SubagentStore`, `welcome`, `textSel` | `theme`, `spinner` ref from footer |
| `ComposerPane` | `ChatInput`, pickers, `palette` | callbacks `onSubmit`, `onCancel`, `onRedraw` |
| `FooterChrome` | `ActivityHandler`, `Spinner` | `labelContext()`, `liveJobs()` closures |
| `Overlays` | `permAskState`, `continueAskState` | `activity` ref, reply callbacks |
| `Submitter` | `BashRunner` | `Controller`, `Bus`, `CommandRegistry`, pane refs |

**Hard rule:** no `*Editor` / `*Shell` back-pointers on handlers. Cross-domain work uses injected refs, callbacks, or `Bus.Publish`.

### Message routing (after refactor)

| `controller.Msg` | Handler |
| ---------------- | ------- |
| `SessionEventMsg`, `JobProgressMsg` | `TranscriptPane` |
| `SubmitMsg`, `CancelStreamMsg` | `Submitter` |
| `PermissionAskMsg`, `PermissionDismissMsg`, `ContinueAskMsg`, `ContinueDismissMsg` | `Overlays` |
| `SetActivityMsg`, `ClearIfActivityMsg`, `UpdateAvailableMsg`, `HookSessionEffectsMsg` (status) | `FooterChrome` |
| `MentionResultsMsg`, `BranchLabelMsg` | `ComposerPane` |
| `HookCommandResultMsg` | `HookCommands` |
| `RedrawMsg` | no-op (redraw already scheduled) |

`Shell.drainBus()` dispatches; `Draw()` calls `drainBus()` first (same as today).

---

## Slices

Each slice is one PR. After every slice: `make test`, `make fmt`, manual smoke `make build && ./phi tui` (or `phi` default).

```text
Slice 1 ──► Slice 2 ──► Slice 3
                │
Slice 4 ◄───────┘ (needs Submitter.StreamActive)
Slice 5 (rename + wire cmd) depends on 1–4
Slice 6 (command deps cleanup) optional, after 5
```

---

### Slice 1 — `TranscriptPane` extraction ✅

**Status:** Done.

**In scope**

- Add `internal/tui/transcript_pane.go` (+ tests mirroring existing transcript/editor tests where practical).
- Move fields: `snap`, `list`, `listIDs`, `mapper`, `subagents`, `welcome`, `sel`, `listH`, `lastListSurf`.
- Move methods: `syncThread`, `applySessionEvent`, `applyAgentToolData`, copy/selection helpers from `copy.go` (or keep `copy.go` but attach to pane).
- `TranscriptPane` API (minimum):
  - `ApplySession(session.Event)`
  - `ApplyJobProgress(job.Progress) bool`
  - `Sync()`, `Snapshot() session.Snapshot`
  - `IsStreaming() bool`, `LastCopyText() string`, `StickToBottomIfPinned()`
  - `Draw(ctx, height)`, `Handle(ctx, ev)` for scroll/selection
  - `Clear()`, `LoadSnapshot(session.Snapshot)` for later session commands
- `Editor` keeps delegating: `editor.transcript.Apply…`, `editor.syncThread()` → one-liner.

**Out of scope**

- `Submitter`, `ComposerPane`, `Shell` rename, `cmd` changes.
- Moving `handleSubmit` / bash / overlays.

**Files**

| Action | Path |
| ------ | ---- |
| add | `internal/tui/transcript_pane.go` |
| add | `internal/tui/transcript_pane_test.go` (Sync / ApplySession smoke) |
| modify | `internal/tui/editor.go` (embed `*TranscriptPane`, delegate) |
| modify | `internal/tui/layout.go`, `copy.go` (call through pane) |

**Acceptance**

- [ ] `Editor` no longer declares `snap`, `list`, `mapper`, `subagents`, `listIDs`, `welcome`, `sel` directly.
- [ ] Streaming transcript, sub-agent nested rows, text copy selection unchanged.
- [ ] `make test ./internal/tui/...` green.

**Commit:** `refactor(tui): extract TranscriptPane from Editor`

---

### Slice 2 — `Submitter` + `BashRunner` ✅

**Status:** Done.

**Goal:** Own submit / cancel / slash / bash in one handler (panda `Submitter`). Remove `BashMode{e *Editor}`.

**In scope**

- Add `internal/tui/submitter.go`, refactor `bash.go` → `BashRunner` with explicit deps (`TranscriptPane`, `ComposerPane` stubs OK until Slice 3 — use narrow interfaces or temporary `Editor` bridge).
- Move from `editor.go`: `handleSubmit`, `handleSlash`, `handleCancel`, `streamActive`, `isBusy` (bash part).
- `SubmitterDeps` struct (documented fields, not a vague `Deps` bag).
- `Editor` wires `Chat.OnSubmit` → `bus.Publish(SubmitMsg)` unchanged; `Update(SubmitMsg)` → `submitter.Apply`.
- `BashRunner` publishes session events through `TranscriptPane.ApplySession` (or `Editor` bridge).

**Out of scope**

- Extracting `ComposerPane` (bash may still touch `editor.Chat` via a thin `ComposerView` interface).
- Session/hook commands refactor.

**Temporary interface (allowed until Slice 3)**

```go
type composerInput interface {
    Value() string
    Clear()
    HideCompleters()
    // … minimal surface BashRunner + Submitter need
}
```

**Acceptance**

- [ ] No `BashMode struct { e *Editor }`.
- [ ] `!cmd`, `/slash`, agent submit, Esc cancel, stream cancel behave as before.
- [ ] `make test` green.

**Commit:** `refactor(tui): extract Submitter and BashRunner`

---

### Slice 3 — `ComposerPane` ✅

**Status:** Done.

**Goal:** Input area owns chat + mention + slash + palette + `InputRouter` logic.

**In scope**

- Add `internal/tui/composer_pane.go`.
- Move: `Chat`, `mention`, `slash`, `palette`, `mentionGen`, all of `input_router.go` logic.
- Move theme hooks that touch composer widgets (`applyTheme` composer section, `BranchLabelMsg`, `MentionResultsMsg`).
- `ComposerPane` callbacks: `OnSubmit`, `OnCancel`, `OnRedraw` (injected by `Editor` / later `Shell`).
- Delete `internal/tui/input_router.go` or reduce to re-export.
- Update `Submitter` / `BashRunner` to depend on `*ComposerPane` instead of `composerInput` bridge.

**Out of scope**

- Footer, overlays, `Shell` rename.

**Acceptance**

- [ ] No `InputRouter struct { e *Editor }`.
- [ ] Ctrl+K palette, `/` slash picker, `@` mention, composer submit unchanged.
- [ ] `make test` green.

**Commit:** `refactor(tui): extract ComposerPane`

---

### Slice 4 — `FooterChrome` + `Overlays` ✅

**Status:** Done.

**Goal:** Footer and modal ask UIs no longer live on `Editor`.

**In scope**

- Add `internal/tui/footer.go` — move `activity`, `spin`, `updateHint`, `hookStatus`, `contextWindow`, `lastUsage`, `updateTokenDisplay`, footer draw from `layout.go`.
- Add `internal/tui/overlays.go` — move `permAskState`, `continueAskState`, `permission_ask.go`, `continue_ask.go` logic; `Active()`, `HandleKey`, `Draw`, `Apply(Msg)`.
- `FooterChrome` gets `SetLabelContext(func() session.Snapshot)` and `SetLiveJobs(func() int)`.
- `EditorLayout` (still on `Editor`) uses pane refs for bottom area: overlays replace composer when active.

**Out of scope**

- `Shell` rename, `cmd` changes.
- `SessionCommands` / `HookCommands` dep injection (Slice 6).

**Acceptance**

- [x] Permission / continue ask overlays, footer spinner, token display, update hint unchanged.
- [x] `Editor` no longer holds `permAsk`, `continueAsk`, `activity`, `spin`, `hookStatus`, `updateHint` directly.
- [x] `make test` green.

**Commit:** `refactor(tui): extract FooterChrome and Overlays`

---

### Slice 5 — `Shell` root + `cmd` wiring

**Goal:** Replace public `Editor` with thin `Shell`; align assembly with panda `InteractiveMode`.

**In scope**

- Add `internal/tui/shell.go`, `shell_layout.go`, `shell_dispatch.go`.
- `NewShell(app, bus, ctrl, cmds, vx, theme, cwd, model, …)` — construct panes in dependency order (see doc section “Assembly order”).
- `Shell` implements `components.Widget` (`Draw`, `Handle`, `RequestRedraw`).
- `cmd/main.go`: `tui.NewEditor` → `tui.NewShell`; `redraw.Bind(shell.RequestRedraw)`.
- Keep `type Editor = Shell` or deprecated alias one release if external importers exist (likely none — check `grep NewEditor`).
- Move `StartUpdateCheck`, `StartBranchWatch` onto `Shell`.
- Delete hollow `Editor` struct / shrink `editor.go` to alias or remove.

**Out of scope**

- `SessionCommands` / `HookCommands` cleanup (Slice 6).
- CHANGELOG entry under `[Unreleased]` (user-visible: none if CLI unchanged).

**Assembly order**

```go
footer     := NewFooterChrome(...)
transcript := NewTranscriptPane(theme, footer.Spinner())
composer   := NewComposerPane(theme, model, cwd)
overlays   := NewOverlays(footer.Activity(), ...)
submitter  := NewSubmitter(SubmitterDeps{...})
composer.SetOnSubmit(...)
composer.SetOnCancel(submitter.Cancel)
footer.SetLabelContext(transcript.Snapshot)
footer.SetLiveJobs(ctrl.LiveJobCount)
shell      := NewShell(ShellConfig{...})
```

**Acceptance**

- [ ] `Editor` type removed or aliased to `Shell` with comment.
- [ ] `Shell` struct has ≤ ~12 fields (app, bus, vx, theme, toast, panes, layout).
- [ ] Full interactive smoke: submit, stream, permission ask, `/clear`, Ctrl+K.
- [ ] `make test && make lint` green.

**Commit:** `refactor(tui): replace Editor with Shell root widget`

---

### Slice 6 — Command handlers (optional cleanup)

**Goal:** Remove last `*Editor` back-pointers from command/session/hook helpers.

**In scope**

- Rename `session_actions.go` → `session_cmds.go`; `SessionActions` → `SessionCommands` with injected deps.
- `HookCommands`: replace `e *Editor` with `registry`, `ctrl`, `submitter`, `composer`, `bus`, `toast`.
- `commandContext()` lives on `Submitter` or small `CommandBridge` type.
- Update `internal/tui/doc.go` and `doc/project-layout.md` (one line in design docs table).

**Out of scope**

- New features, Bus/Controller changes.

**Acceptance**

- [ ] `grep '*Editor'` in `internal/tui` returns zero (except tests/comments).
- [ ] Hook slash commands, `/sessions`, `/resume`, `/clear` unchanged.
- [ ] `make test` green.

**Commit:** `refactor(tui): inject deps into session and hook commands`

---

## Verification checklist (every slice)

```bash
make fmt
make test
make lint   # if touching many files
make build
./phi       # smoke: type a message, cancel stream, exercise one slash command
```

## Rollback

Each slice is independently revertable. Prefer additive files first, then switch `Editor` to delegate, then delete moved code in the same PR (not across PRs).

## Non-goals

- Codex-style mega `AppEvent` enum or overlay stack framework.
- Moving `Controller` / `Engine` into `tui`.
- Changing `controller.Bus` coalescing or `transcript.Mapper` algorithm (unless a slice bugfix requires it).
- Feature flags — each slice keeps default TUI fully working.

## File map (end state)

```text
internal/tui/
├── shell.go
├── shell_layout.go
├── shell_dispatch.go
├── transcript_pane.go
├── composer_pane.go
├── footer.go
├── overlays.go
├── submitter.go
├── bash.go                 # BashRunner
├── session_cmds.go
├── hook_cmds.go
├── commands.go
├── registry.go
├── copy.go                 # or merged into transcript_pane
├── branch.go
├── tokens.go
├── controller/
└── transcript/
```

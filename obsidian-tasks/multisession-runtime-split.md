---
id: multisession-runtime-split
title: Разделить Controller на Process-runtime, Workspace (на cwd) и per-session Controller
status: done
priority: high
model_level: very_high
task_type: refactor
parent_id: multisession-mode
tags:
    - cozyphi
    - tui
    - controller
    - multisession
branch: refactor/multisession-runtime-split
worktree_path: .worktrees/multisession-runtime-split
acceptance_criteria:
    - Есть Runtime (процесс) и Workspace (на cwd) с явным владением ресурсами; Controller per-session и получает их параметрами; два Controller'а на одном Workspace работают в тесте одновременно
    - cmd/main.go и headless run собирают всё через Runtime/Workspace; поведение одной сессии не изменилось (хуки session_start/shutdown, порядок Close, бюджеты)
    - Нет обратных указателей на Editor, нет Deps-мешков; doc/tui.md раздел про Controller обновлён
    - go test -race ./internal/tui/controller/... зелёный; make fmt-check lint test в worktree зелёные
verification_plan:
    - 'go test -race ./internal/tui/controller/... ./cmd/... в worktree; тест: два Controller на одном Workspace, оба Close, shared закрыт один раз'
    - 'Живой smoke: cozyphi, /resume, /clear, watch, sub-agent, quit — как раньше; cozyphi run -p работает'
    - golangci-lint run на изменённых пакетах один раз перед коммитом
created_at: "2026-09-04T07:31:55.425794Z"
updated_at: "2026-09-05T16:47:35.595726Z"
---

## Body

**Контекст:** `controller.NewController(bus, proj, cwd, resumePath, histories)` (internal/tui/controller/controller.go) сам поднимает всё: providers → opencode → plan defaults → memory.Open → watch.New → tasks.Discover → gate → lsp.Open → hooks → NewJobManager → mcp.LoadPool → newEngine. Чтобы в процессе жили несколько сессий (в т.ч. в разных проектах), общие ресурсы должны создаваться один раз и передаваться параметрами.

**Что сделать:**
1. `controller.Runtime` (процесс): providers, opencode-клиент, jobs manager, usage history, close-бюджет; конструктор `NewRuntime(proj, histories...)`, `Close()`.
2. `controller.Workspace` (на cwd / git-root): project config, mcp pool, lsp manager, memory store, tasks registry, hooks manager; `runtime.Workspace(cwd)` создаёт лениво и кеширует, `Close()` при закрытии последней сессии проекта (refcount).
3. `NewController(bus, rt, ws, opts SessionOpts)` — только per-session: engine, stream, gate, mode, watches, plan runtime, watchQueue, session hooks. `Close()` закрывает только своё (stream, watches, unsub jobs), не трогает shared. Сигнатуры существующих методов Controller (Resume/Clear/Submit/…) сохраняются.
4. cmd/main.go: `rt := NewRuntime(...)`, `ws := rt.Workspace(cwd)`, `ctrl := NewController(bus, rt, ws, ...)`, `defer rt.Close()`; `cozyphi run` (headless) тоже через Runtime/Workspace, если использует Controller/те же инициализаторы.
5. harnesssettings.Open принимает то, что нужно от Runtime (plan runtime — общий на процесс).
6. Поведение single-session не меняется: те же хуки, порядок shutdown, те же тосты.

**Границы:** никакого мультиплексора и панели — только разделение владения. Без `XxxDeps`-мешков: конструкторы с параметрами.

**Blocked by:** —

**Started (2026-09-05).** Implementation approved for the interactive-child-sessions-design contract (ledger commit 6aa5898). First implementation slice: process/workspace/session ownership only. Main baseline 6aa589845ecc40e67ddc9f52a8d3b3c58c4f0436. Unrelated go.sum and three untracked task notes are preserved. Mutable plans/approvals/evidence stay session-local; shared plangate.Runtime is immutable policy only. Canonical workdir/config identity must not collapse distinct worktrees. Code and focused checks run in .worktrees/multisession-runtime-split; high is the maximum supported effort.

**Note (2026-09-05).** Runtime worktree now has explicit Runtime/Workspace/NewSession assembly; single-session NewController owns a private runtime for compatibility. Shared services outlive borrowed Controllers; Close cancels only owned jobs and defers workspace teardown until actual runners exit. Shared Manager uses per-engine immutable JobRunnerFactory; progress carries authoritative ParentID and is filtered before per-session Bus delivery. History writer/corpus shared via independent NewCursor. Initial public runtime two-session/cancellation and progress-routing tests pass under race; full changed controller/history/job package tests passed before latest snapshot wiring. Independent ownership review running. Additional prerequisite findings registered: plan-schema-copy-isolation and mcp-workspace-process-cwd (being implemented in this runtime slice), job-recovery-process-ownership (open for hardening). No claim of interactive UI completion; no final lint/commit yet. NOTES.md remains ignored/local.

**Note (2026-09-05).** Runtime follow-up review found no must-fix defects. Added immutable live OwnerID distinct from persisted ParentID, scoped tool access/progress/count/teardown, cancellation-aware constructors outside admission lock, eager role model snapshots, headless rebound runner snapshots, permanent MCP close with explicit canonical cwd, and isolated plan schema injection. Scoped race tests for controller/history/job/agenttool/plangate/MCP passed; final changed-package tests and the one permitted scoped lint are running. doc/tui.md and CHANGELOG updated. Accepted design supersedes original refcount wording: workspaces remain until process shutdown; headless remains a direct Engine with private manager and explicit workspace assembly, not a TUI Controller. Latest user constraint replaces whole-repo gates with scoped checks. Live terminal smoke remains for full interactive UI delivery, not claimed by this prerequisite. Main advanced independently to 33b82fd; integration must preserve its picker/planning work and unrelated working edits.

**Done (2026-09-05).** Implemented in 0d903f9, integrated current main in 18462f1 and merged locally to main. Explicit Runtime/Workspace ownership, isolated Controllers/history cursors, immutable job OwnerID with scoped access/progress/teardown, eager engine/headless runner snapshots and cancellation-safe constructors are connected. doc/tui.md and CHANGELOG updated. Independent follow-up review found no blocking defects. Full changed-package race tests passed; single scoped lint reported 11 mechanical findings, all addressed with targeted post-fix race regressions (lint deliberately not rerun under user limit). Integration tests ./cmd ./internal/tui/controller ./internal/tui/editor passed after bringing current main into the task worktree. No live terminal smoke or interactive-child UI completion claimed: retained Views/lifecycle/inbox are subsequent tasks. NOTES copied to ignored main NOTES.md; unrelated edits preserved; no push.

## Acceptance Criteria

- Есть Runtime (процесс) и Workspace (на cwd) с явным владением ресурсами; Controller per-session и получает их параметрами; два Controller'а на одном Workspace работают в тесте одновременно
- cmd/main.go и headless run собирают всё через Runtime/Workspace; поведение одной сессии не изменилось (хуки session_start/shutdown, порядок Close, бюджеты)
- Нет обратных указателей на Editor, нет Deps-мешков; doc/tui.md раздел про Controller обновлён
- go test -race ./internal/tui/controller/... зелёный; make fmt-check lint test в worktree зелёные

## Verification Plan

1. go test -race ./internal/tui/controller/... ./cmd/... в worktree; тест: два Controller на одном Workspace, оба Close, shared закрыт один раз
2. Живой smoke: cozyphi, /resume, /clear, watch, sub-agent, quit — как раньше; cozyphi run -p работает
3. golangci-lint run на изменённых пакетах один раз перед коммитом

---
id: multisession-runtime-split
title: Разделить Controller на Process-runtime, Workspace (на cwd) и per-session Controller
status: todo
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
updated_at: "2026-09-04T07:31:55.425794Z"
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

## Acceptance Criteria

- Есть Runtime (процесс) и Workspace (на cwd) с явным владением ресурсами; Controller per-session и получает их параметрами; два Controller'а на одном Workspace работают в тесте одновременно
- cmd/main.go и headless run собирают всё через Runtime/Workspace; поведение одной сессии не изменилось (хуки session_start/shutdown, порядок Close, бюджеты)
- Нет обратных указателей на Editor, нет Deps-мешков; doc/tui.md раздел про Controller обновлён
- go test -race ./internal/tui/controller/... зелёный; make fmt-check lint test в worktree зелёные

## Verification Plan

1. go test -race ./internal/tui/controller/... ./cmd/... в worktree; тест: два Controller на одном Workspace, оба Close, shared закрыт один раз
2. Живой smoke: cozyphi, /resume, /clear, watch, sub-agent, quit — как раньше; cozyphi run -p работает
3. golangci-lint run на изменённых пакетах один раз перед коммитом

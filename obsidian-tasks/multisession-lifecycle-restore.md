---
id: multisession-lifecycle-restore
title: 'Жизненный цикл сессий: закрытие с подтверждением, «Недавние», восстановление набора открытых сессий при старте'
status: todo
priority: medium
model_level: high
task_type: feature
parent_id: multisession-mode
tags:
    - cozyphi
    - tui
    - multisession
branch: feature/multisession-lifecycle-restore
worktree_path: .worktrees/multisession-lifecycle-restore
acceptance_criteria:
    - Закрытие бегущей сессии требует подтверждения и корректно останавливает стрим; закрытая появляется в «Недавних» и переоткрывается
    - Набор открытых сессий, порядок и активная восстанавливаются при старте; sessions.restore=false отключает; -c/--resume работают поверх набора
    - Quit закрывает все сессии в общий бюджет, хуки session_shutdown срабатывают на каждую
    - Тесты UIState round-trip и restore с пропавшим файлом; make fmt-check lint test в worktree зелёные
verification_plan:
    - go test ./internal/project/... ./internal/tui/sessions/... ./cmd/... в worktree
    - 'Живой smoke: открыть 3 сессии в 2 проектах, закрыть одну бегущую, выйти, запустить — набор восстановлен; удалить jsonl одной, запустить — тост и остальные на месте'
    - golangci-lint run на изменённых пакетах один раз перед коммитом
created_at: "2026-09-04T07:31:55.433694Z"
updated_at: "2026-09-04T07:31:55.433694Z"
---

## Body

**Контекст:** `project.UIState`/`MutateUIState` (internal/project/uistate.go) хранят настройки UI; `cmd/tui.go` — `--continue/--resume`. Codex и Claude Code восстанавливают открытые треды при запуске.

**Что сделать:**
1. Закрытие (`x` в панели, /close, палитра, Ctrl+W занят — не назначать): бегущая сессия → подтверждение «Сессия бежит, остановить и закрыть?»; далее cancel stream → `Controller.Close()` с бюджетом → Workspace refcount; последняя сессия не закрывается — вместо этого /clear-семантика (новая пустая).
2. Закрытая сессия остаётся на диске и появляется в «Недавние» панели (`◌`), переоткрывается Enter/пикером; список ограничен последними 20 на проект.
3. Персист: `UIState.OpenSessions []{File, Cwd}` + `ActiveSession` + порядок, обновляется при open/close/switch (debounce); `sessions.restore` в конфиге (по умолчанию true).
4. Старт: при restore открываются сохранённые сессии (несуществующие файлы пропускаются с одним тостом), активная — сохранённая; `-c`/`--resume` добавляют/активируют указанную поверх набора; без restore — как сейчас. Открытие происходит без запуска стримов; модель/эффорт каждой — из её заголовка/last-model правил как у Resume.
5. Quit: Close всех сессий последовательно с общим бюджетом (уже открытые бюджеты Runtime), порядок хуков session_shutdown на каждую.
6. doc/tui.md, doc/config (sessions.restore); CHANGELOG.

**Blocked by:** multisession-registry, multisession-sessions-panel, multisession-hotkeys

## Acceptance Criteria

- Закрытие бегущей сессии требует подтверждения и корректно останавливает стрим; закрытая появляется в «Недавних» и переоткрывается
- Набор открытых сессий, порядок и активная восстанавливаются при старте; sessions.restore=false отключает; -c/--resume работают поверх набора
- Quit закрывает все сессии в общий бюджет, хуки session_shutdown срабатывают на каждую
- Тесты UIState round-trip и restore с пропавшим файлом; make fmt-check lint test в worktree зелёные

## Verification Plan

1. go test ./internal/project/... ./internal/tui/sessions/... ./cmd/... в worktree
2. Живой smoke: открыть 3 сессии в 2 проектах, закрыть одну бегущую, выйти, запустить — набор восстановлен; удалить jsonl одной, запустить — тост и остальные на месте
3. golangci-lint run на изменённых пакетах один раз перед коммитом

---
id: settings-sidebar-general
title: 'Settings: размер контекста сессии (sidebar) и лимит контекста агентов (General)'
status: done
priority: high
task_type: feature
branch: feature/settings-sidebar-general
worktree_path: .worktrees/settings-sidebar-general
acceptance_criteria:
    - В сайдбаре настроек можно менять размер контекста текущей сессии отдельно для основной сессии и для агентов; изменение применяется в текущей сессии и не сохраняется между сессиями (на диск не пишется)
    - В настройках General есть ограничение контекста для агентов; оно персистентно и применяется к сессиям агентов
    - Scoped-гейты по затронутым пакетам зелёные, CHANGELOG пополнен
verification_plan:
    - go test по затронутым пакетам (settings, controller, components)
    - gofmt -l + golangci-lint fmt по изменённым пакетам
    - Один golangci-lint run по изменённым пакетам перед коммитом
    - 'Ручная проверка в TUI: контролы меняют бюджет, после рестарта значения сброшены'
created_at: "2026-09-05T20:33:41.026189Z"
updated_at: "2026-09-05T21:14:36.577778Z"
---

## Body

**Что:** в сайдбаре настроек добавить управление размером контекста текущей сессии — отдельно для основной сессии и для агентов. Значения живyт только в рамках сессии, между сессиями не сохраняются. Отдельно в настройки **General** добавить постоянное ограничение контекста для агентов.

**Почему:** пользователю нужно на ходу поджимать/расширять контекстный бюджет сессии (основной и агентных) без правки конфигов; для агентов нужен сохраняемый потолок по умолчанию.

**Границы:** сессионные значения не пишутся в настройки; General-значение персистентно. UI — dumb-виджеты в internal/components, проводка в internal/tui.

**Note (2026-09-06).** 2026-09-04: implemented and merged. Sidebar settings tab: session-only context controls (main window + agent ceiling) in digit entries — click/enter to edit, Enter/Space commits, Escape cancels, nothing persisted. Settings modal General tab: persisted agents.context_limit row (digit entry, 0 = unlimited). Backend: engine contextOverride + EngineRunner child window narrowing, harnesssettings AgentContextLimit with validation, controller atomics + EffectiveContextWindow, sessions view wiring. Commit ae691af on feature/settings-sidebar-general, merged --no-ff to main. Scoped gates green (lint parity with main). CHANGELOG under [Unreleased].

**Done (2026-09-06).** Landed on main via merge 6eafb53 (feature commit ae691af, branch feature/settings-sidebar-general). Sidebar settings tab: session-only context window + agent ceiling digit entries (not persisted). General tab: persisted agents.context_limit applied to every sub-agent at spawn. Scoped gates green, CHANGELOG updated, worktree and branch cleaned up.

## Acceptance Criteria

- В сайдбаре настроек можно менять размер контекста текущей сессии отдельно для основной сессии и для агентов; изменение применяется в текущей сессии и не сохраняется между сессиями (на диск не пишется)
- В настройках General есть ограничение контекста для агентов; оно персистентно и применяется к сессиям агентов
- Scoped-гейты по затронутым пакетам зелёные, CHANGELOG пополнен

## Verification Plan

1. go test по затронутым пакетам (settings, controller, components)
2. gofmt -l + golangci-lint fmt по изменённым пакетам
3. Один golangci-lint run по изменённым пакетам перед коммитом
4. Ручная проверка в TUI: контролы меняют бюджет, после рестарта значения сброшены

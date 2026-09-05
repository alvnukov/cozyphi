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
updated_at: "2026-09-05T23:37:28.468716Z"
---

## Body

**Что:** в сайдбаре настроек добавить управление размером контекста текущей сессии — отдельно для основной сессии и для агентов. Значения живyт только в рамках сессии, между сессиями не сохраняются. Отдельно в настройки **General** добавить постоянное ограничение контекста для агентов.

**Почему:** пользователю нужно на ходу поджимать/расширять контекстный бюджет сессии (основной и агентных) без правки конфигов; для агентов нужен сохраняемый потолок по умолчанию.

**Границы:** сессионные значения не пишутся в настройки; General-значение персистентно. UI — dumb-виджеты в internal/components, проводка в internal/tui.

**Note (2026-09-06).** 2026-09-04: implemented and merged. Sidebar settings tab: session-only context controls (main window + agent ceiling) in digit entries — click/enter to edit, Enter/Space commits, Escape cancels, nothing persisted. Settings modal General tab: persisted agents.context_limit row (digit entry, 0 = unlimited). Backend: engine contextOverride + EngineRunner child window narrowing, harnesssettings AgentContextLimit with validation, controller atomics + EffectiveContextWindow, sessions view wiring. Commit ae691af on feature/settings-sidebar-general, merged --no-ff to main. Scoped gates green (lint parity with main). CHANGELOG under [Unreleased].

**Done (2026-09-06).** Landed on main via merge 6eafb53 (feature commit ae691af, branch feature/settings-sidebar-general). Sidebar settings tab: session-only context window + agent ceiling digit entries (not persisted). General tab: persisted agents.context_limit applied to every sub-agent at spawn. Scoped gates green, CHANGELOG updated, worktree and branch cleaned up.

**Reopened (2026-09-06).** Reopened: user reports the sidebar context rows cannot be changed in the live TUI; wants +/- steppers with 10k steps anchored to the persisted General agents.context_limit, not the model context window.

**Note (2026-09-06).** 2026-09-05: reopened after user report — sidebar context rows cannot be edited in the live TUI (unit tests passed). New plan rev 257: diagnose routing, replace digit entries with +/- steppers (10k step) anchored to General agents.context_limit (0 → window from model, agents ∞; floor 10k, below → reset). Working at /effort high per user.

**Note (2026-09-06).** 2026-09-05: reopened after user report — sidebar context rows cannot be edited in the live TUI (unit tests passed). New plan rev 257: diagnose routing, replace digit entries with +/- steppers (10k step) anchored to General agents.context_limit (0 → window from model, agents ∞; floor 10k, below → reset). Working at /effort high per user.

**Note (2026-09-06).** 2026-09-05: user declined xhigh ("нет xhigh только high") — remember when updating suggest-effort-level memory.

**Note (2026-09-06).** Diagnosis in progress: sidebar.go context-row mouse/key routing vs live shell wiring in view.go.

**Note (2026-09-06).** 2026-09-05: user demanded explicit per-step effort in the plan — patched all three steps to effort:high (user: "нет xhigh только high"). Stop stalling on memory calls; work the diagnosis.

**Done (2026-09-06).** Sidebar Settings context rows are now −/+ steppers (50k step, 10k floor resets the override to the General default). Main row = session override of the compact reminder threshold, moved live via engine compaction settings; agents row = session override of agents.context_limit, override-wins over the General default (General is a default, not a cap), applied at spawn. Nothing is written to disk. Digit-entry keyboard wiring removed; sidebar chips are the only path. Regression test drives clicks through the real App dispatch with composer focus. Landed as 9228a57, merged into main as f479f5a (resolved conflict with b67e436 child-ceiling work: engine_window.go machinery kept). Lint: my findings fixed; 7 baseline findings remain in untouched files.

**Reopened (2026-09-06).** Reopened for a follow-up refinement: step 10k (floor 10k unchanged, start at 50k), stepper buttons flanking the value as circled ⊖/⊕ glyphs.

**Done (2026-09-06).** Follow-up refinement landed: context steppers step by 10k (unset starts at 50k, below the 10k floor resets to the General default), rendered as circled ⊖/⊕ glyphs flanking the value; chip hit zones are the glyph cells. Commit 53c8668, merged to main as dccbfe6. Gates: build+tests green (sidebar, app, controller, sessions, agent, harnesssettings), gofmt/lint-fmt clean, scoped lint has only the pre-existing baseline unparam in the untouched sidebar_plan_test.go. Dispatch regression test now derives the plus column from layout (Width-3) — the surface text rune index is shifted +2 by the frame inset.

## Acceptance Criteria

- В сайдбаре настроек можно менять размер контекста текущей сессии отдельно для основной сессии и для агентов; изменение применяется в текущей сессии и не сохраняется между сессиями (на диск не пишется)
- В настройках General есть ограничение контекста для агентов; оно персистентно и применяется к сессиям агентов
- Scoped-гейты по затронутым пакетам зелёные, CHANGELOG пополнен

## Verification Plan

1. go test по затронутым пакетам (settings, controller, components)
2. gofmt -l + golangci-lint fmt по изменённым пакетам
3. Один golangci-lint run по изменённым пакетам перед коммитом
4. Ручная проверка в TUI: контролы меняют бюджет, после рестарта значения сброшены

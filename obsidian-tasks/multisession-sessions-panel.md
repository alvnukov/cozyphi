---
id: multisession-sessions-panel
title: 'Левая панель сессий: группы по проектам, статусы, номера, фильтр, фокус-режим, мышь, ширина в ui.json'
status: todo
priority: high
model_level: high
task_type: feature
parent_id: multisession-mode
tags:
    - cozyphi
    - tui
    - multisession
branch: feature/multisession-sessions-panel
worktree_path: .worktrees/multisession-sessions-panel
acceptance_criteria:
    - Панель показывает проекты → сессии с номером, статусом, заголовком, возрастом, «Недавние» закрытые; активная строка выделена акцентным цветом
    - Ширина перетаскивается, ширина и видимость сохраняются в ui.json; при ширине терминала < 100 панель не резервирует место и открывается поверх
    - 'Фокус-режим: ↑↓ Enter n N r x / Esc работают, подсказки в футере; мышь: клик переключает, колесо скроллит'
    - Тесты рендера (golden/строковые) на статусы, усечение, узкий терминал; make fmt-check lint test в worktree зелёные
verification_plan:
    - go test ./internal/tui/sessionpane/... ./internal/tui/editor/... в worktree
    - 'Живой smoke на 80 и 140 колонках: панель, drawer, перетаскивание, r/x/n, клик мышью'
    - golangci-lint run на изменённых пакетах один раз перед коммитом
created_at: "2026-09-04T07:31:55.427967Z"
updated_at: "2026-09-04T07:31:55.427967Z"
---

## Body

**Контекст:** правый сайдбар (internal/tui/sidebar) — образец: `ReserveWidth`, `ConfigureWidth/ConfigureVisibility` с сохранением в ui.json (`project.UIState`), `HandleGlobalMouse`, `HandleScrollKey`. Модель сессий — `sessions.Registry` (multisession-registry).

**Что сделать:**
1. Пакет `internal/tui/sessionpane`: тупой view над `Registry` + список «Недавние» (`session.ListSessions` по проектам, закрытые, `◌`). Дерево: проект (имя каталога, короткий путь `pathutil.ShortPath`, сворачивание) → строки сессий `N ⟨статус⟩ заголовок ····· возраст`; номера 1..9 сквозные в визуальном порядке (это же цели Alt+N). Активная строка — акцентный цвет сессии и маркер `▸`. Статусы: `◐`+elapsed, `?`, `●`+счётчик, `✗`, `⏸n`, пусто, `◌`; спиннер в активной бегущей.
2. Геометрия: ширина по умолчанию 28, перетаскивание границы мышью как у правого сайдбара, `ReserveWidth(total)` возвращает 0 при total < 100 — тогда панель открывается поверх (drawer) по фокус-команде; ширина и видимость — `UIState.SessionPaneWidth/Hidden` через `MutateUIState`. Editor.Draw сдвигает X-начало list/chat/footer на ширину панели.
3. Фокус-режим (вход по команде из multisession-hotkeys): ↑↓ по строкам, Enter — переключить/открыть закрытую, n — новая, N — новая в каталоге (делегирует Registry, UI ввода пути — в multisession-projects), r — переименовать (inline-ввод → /rename семантика), x — закрыть (подтверждение для бегущей), / — фильтр по заголовку/пути, Esc — назад в композер; строка фильтра сверху; подсказки клавиш в футере (`keys.ScopeSessions`).
4. Мышь: клик по строке переключает, клик по заголовку проекта сворачивает, колесо скроллит.
5. Заголовок сессии в строке усечён с `…`, при hover/фокусе на строке полный заголовок и cwd показываются в футере.
6. doc/tui.md: раздел про панель; CHANGELOG.

**Границы:** без глобальных чордов (multisession-hotkeys), без тостов о фоне (multisession-background-attention), без ввода пути проекта (multisession-projects).

**Blocked by:** multisession-registry

## Acceptance Criteria

- Панель показывает проекты → сессии с номером, статусом, заголовком, возрастом, «Недавние» закрытые; активная строка выделена акцентным цветом
- Ширина перетаскивается, ширина и видимость сохраняются в ui.json; при ширине терминала < 100 панель не резервирует место и открывается поверх
- Фокус-режим: ↑↓ Enter n N r x / Esc работают, подсказки в футере; мышь: клик переключает, колесо скроллит
- Тесты рендера (golden/строковые) на статусы, усечение, узкий терминал; make fmt-check lint test в worktree зелёные

## Verification Plan

1. go test ./internal/tui/sessionpane/... ./internal/tui/editor/... в worktree
2. Живой smoke на 80 и 140 колонках: панель, drawer, перетаскивание, r/x/n, клик мышью
3. golangci-lint run на изменённых пакетах один раз перед коммитом

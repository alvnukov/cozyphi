---
id: hover-tooltip
title: Ховер-тултипы в TUI (dwell, верхний слой)
status: done
priority: high
task_type: feature
branch: feature/hover-tooltip
worktree_path: .worktrees/hover-tooltip
acceptance_criteria:
    - Подсказка появляется при dwell на любом подсвеченном контроле и исчезает при уходе/смене региона/клике; текст задаётся виджетом
    - 'Без глюков: нет мигания (dwell + перерисовка только на смену состояния), нет наложений (один верхний слой), нет вылезаний за экран (флип/кламп)'
    - 'Idle-кадры 0 байт: без событий мыши перерисовок нет'
    - Scoped go test + один scoped golangci-lint зелёные
    - CHANGELOG под [Unreleased], ledger done, подписанный conventional коммит в ветке задачи
verification_plan:
    - go build + go test по изменённым пакетам
    - golangci-lint run по изменённым пакетам — один раз перед коммитом
    - 'Ручная проверка в TUI: dwell-показ, флип у краёв, стиль, клики/скролл живы'
created_at: "2026-09-13T11:18:30.796522Z"
updated_at: "2026-09-13T20:45:00Z"
---

## Body

**Что:** всплывающая подсказка (тултип) при ховере на контролах TUI cozyphi — появляется при dwell курсора (~500 мс), исчезает при уходе/смене региона/клике.

**Контекст:** Phase 1 (UI/UX convenience), продолжение задачи hover-button-highlight (ховер-подсветка уже есть). Инварианты те же: виджеты dumb, draw-on-demand через Wake/WakeIn, idle-кадры 0 байт.

**Границы:** тултип — верхний слой поверх композиции кадра (один владелец, нет z-fighting); контент через существующий hover-шов (виджет отвечает текстом для x,y); флип/кламп у краёв терминала; стиль ask-панелей (бордер + BackgroundElement). Клики/скролл/жесты не меняются. Гейты только по изменённым пакетам, один scoped lint. Работа в .worktrees/, доставка через PR с разрешения.

## Acceptance Criteria

- Подсказка появляется при dwell на любом подсвеченном контроле и исчезает при уходе/смене региона/клике; текст задаётся виджетом
- Без глюков: нет мигания (dwell + перерисовка только на смену состояния), нет наложений (один верхний слой), нет вылезаний за экран (флип/кламп)
- Idle-кадры 0 байт: без событий мыши перерисовок нет
- Scoped go test + один scoped golangci-lint зелёные
- CHANGELOG под [Unreleased], ledger done, подписанный conventional коммит в ветке задачи

## Verification Plan

1. go build + go test по изменённым пакетам
2. golangci-lint run по изменённым пакетам — один раз перед коммитом
3. Ручная проверка в TUI: dwell-показ, флип у краёв, стиль, клики/скролл живы

**Done (2026-09-13):** реализовано в ветке feature/hover-tooltip. Шов `HoverTooltiper(x,y) (string, bool)` (`internal/components/pointer.go`) + `PointerOwner` — модальный ask владеет мышью и гасит хинт (`Editor.OwnsPointer()` через `Screen().AskOpen()` → `View.AskOpen()` → `overlays.AskOpen()`). Панель `internal/components/tooltip` (`PlaceTooltip`): wrap по словам/глифам, кэп 6 строк, флип/кламп, BackgroundPanel+rounded border, Z:60, Widget nil — клики насквозь; 5 тестов. Движок `internal/components/app/tooltip.go`: dwell 500 мс, re-arm/swap/dismiss+suppress, reconcile при скролле, wake через `WakeIn`; хуки в `app.go` (`NewApp(vx, th)` — сигнатура обновлена по ~15 вызовам); 8 тестов. Контент: блоки tool/diff/turn/agent/compaction/status (fold/unfold зеркалят `PointerShape`), sidebar (контролы, динамические состояния, skill-строки, steppers), footer watch-runs, usagepane reset, session links, `status.Expandable`; ask-опции без тултипа осознанно. Гейты: `gofmt -l` чист, scoped build+test (`./internal/components/... ./internal/tui/... ./cmd/...`) зелёные, один scoped `golangci-lint run`. CHANGELOG под `[Unreleased]`. Ручная проверка в TUI осталась за пользователем.

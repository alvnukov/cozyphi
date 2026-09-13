---
id: hover-button-highlight
title: Ховер-подсветка всех кнопок в TUI
status: done
priority: high
model_level: medium
task_type: feature
branch: feature/hover-button-highlight
worktree_path: .worktrees/hover-button-highlight
acceptance_criteria:
    - Все кнопки TUI подсвечиваются при наведении и возвращают стиль при уходе курсора
    - Клики, скролл и существующие мышиные жесты не меняются
    - Idle-кадры по-прежнему пишут 0 байт (перерисовка только на движение мыши)
    - Scoped go test и один scoped golangci-lint run по изменённым пакетам зелёные
    - 'CHANGELOG: строка под [Unreleased]'
verification_plan:
    - go build + go test по изменённым пакетам
    - golangci-lint run по изменённым пакетам — один раз перед коммитом
    - 'Ручная проверка ховера в TUI: все кнопки подсвечиваются, клики и скролл живы'
created_at: "2026-09-13T10:11:30.257395Z"
updated_at: "2026-09-13T10:43:02.154851Z"
---

## Body

**Что:** ховер-подсветка всех кнопок в TUI cozyphi — при движении мыши кнопка под курсором меняет стиль, при уходе курсора возвращает обычный.

**Контекст:** Phase 1 (UI/UX convenience). Виджеты в `internal/components` остаются dumb, оболочка в `internal/tui`; отрисовка draw-on-demand через `DrawContext.Wake`/`WakeIn`/`WakeAt`, idle-кадры пишут ноль байт — hover обязан вписаться в эти инварианты.

**Границы:** существующее поведение мыши (клики, скролл) не меняется; hover — только визуал. Гейты только по изменённым файлам/пакетам. Работа в `.worktrees/`, доставка через PR.

**Started (2026-09-13).** План одобрен (4 шага: inventory → implement → verify → commit). Начинаю с инвентаризации кнопок и мышиного пайплайна.

**Done (2026-09-13).** Общий конвейер `PointerShape` → `App.updateHover` → `HoverState` → `Hovering`/`ApplyHoverRect`/`HoverTitleRows`; новый `ApplyHoverRect` в `components`. Закрыты все поверхности: заголовки diff/turn-блоков, ⏱-руны футера, reset-кнопки usagepane, сайдбар (табы/approve/settings/чипы/skill-строки через единый `hoverCell`), sessionLink, ask-оверлеи (hover из motion в `HandleAskMouse`, панель не хит-тестится). Мёртвый `layout.Button`/`Clickable` вырезан. Idle-кадры 0 байт: кадр только при смене hover-региона. Scoped go test по 8 пакетам зелёные, один scoped golangci-lint — 0 issues. CHANGELOG под [Unreleased]. Коммит `feat(tui): highlight the control under the pointer` в ветке задачи.

## Acceptance Criteria

- Все кнопки TUI подсвечиваются при наведении и возвращают стиль при уходе курсора
- Клики, скролл и существующие мышиные жесты не меняются
- Idle-кадры по-прежнему пишут 0 байт (перерисовка только на движение мыши)
- Scoped go test и один scoped golangci-lint run по изменённым пакетам зелёные
- CHANGELOG: строка под [Unreleased]

## Verification Plan

1. go build + go test по изменённым пакетам
2. golangci-lint run по изменённым пакетам — один раз перед коммитом
3. Ручная проверка ховера в TUI: все кнопки подсвечиваются, клики и скролл живы

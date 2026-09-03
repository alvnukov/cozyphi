---
id: fix-ctrl-r-placeholder-overlap
title: 'Ctrl+R: плейсхолдер поиска перекрывает найденный текст'
status: done
priority: high
model_level: medium
task_type: bug
tags:
    - tui
    - bug
branch: bug/fix-ctrl-r-placeholder-overlap
worktree_path: .worktrees/fix-ctrl-r-placeholder-overlap
acceptance_criteria:
    - Плейсхолдер поиска не отображается при непустом запросе и не перекрывает найденный текст
    - Поведение остальных мест с тем же виджетом ввода не изменилось
    - make fmt-check lint test зелёный; строка в CHANGELOG [Unreleased]
verification_plan:
    - 'Воспроизвести вручную в TUI: Ctrl+R, ввод запроса — виден только найденный текст'
    - go test по затронутому пакету
    - make fmt-check lint test
created_at: "2026-09-03T22:00:27.002466Z"
updated_at: "2026-09-03T22:15:16.915001Z"
---

## Body

**Что:** В TUI у поиска, вызываемого по Ctrl+R, надпись-плейсхолдер по умолчанию перекрывает найденный текст.

**Воспроизведение:** открыть поиск Ctrl+R, ввести запрос — подсказка по умолчанию остаётся на экране поверх введённого/найденного текста.

**Ожидание:** плейсхолдер виден только при пустом запросе и не рендерится поверх совпадений.

**Started (2026-09-04).** План одобрен, старт шага locate-placeholder-render: локализация причины перекрытия плейсхолдера в поиске Ctrl+R.

**Done (2026-09-04).** Готово: плейсхолдер композера больше не рисуется поверх превью reverse-i-search. Причина — chat_input.go:1078 рисовал плейсхолдер по c.Value == "", а в режиме поиска тело ввода — превью совпадения (или «no matches») при нетронутом пустом черновике. Фикс: условие bodyText == "" && !c.search.active (ffbf8d7, merge 016e95b в main). Тест TestSearchPreviewOwnsTheBody покрывает совпадение и «no matches». Гейт scoped по пакету chat по указанию пользователя: fmt-diff 0 issues, lint 0 issues, go test ok. CHANGELOG [Unreleased] обновлён.

## Acceptance Criteria

- Плейсхолдер поиска не отображается при непустом запросе и не перекрывает найденный текст
- Поведение остальных мест с тем же виджетом ввода не изменилось
- make fmt-check lint test зелёный; строка в CHANGELOG [Unreleased]

## Verification Plan

1. Воспроизвести вручную в TUI: Ctrl+R, ввод запроса — виден только найденный текст
2. go test по затронутому пакету
3. make fmt-check lint test

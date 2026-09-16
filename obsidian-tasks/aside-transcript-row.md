---
id: aside-transcript-row
title: 'Строка aside в ленте: свой цвет, сворачивание, ссылка на якорь'
status: todo
priority: high
model_level: medium
task_type: feature
parent_id: context-window-management
tags:
    - tui
    - transcript
    - aside
    - theme
branch: feature/aside-transcript-row
worktree_path: .worktrees/aside-transcript-row
acceptance_criteria:
    - Aside показывается отдельным цветом в обеих темах, с вопросом в заголовке и ссылкой на якорь, когда он не текущий лист
    - Клик по заголовку сворачивает/разворачивает, hover подсвечивает заголовок, dwell показывает тултип
    - Стрим ответа обновляет строку через tail-патч; после replay строка на том же месте и в том же состоянии по умолчанию
    - Экспорт в markdown содержит aside с пометкой
    - Строка в CHANGELOG под Unreleased
verification_plan:
    - go test ./internal/session/... ./internal/components/block/... ./internal/tui/transcript/... ./internal/components/...
    - 'Ручная проверка в обеих темах: /btw, свернуть, развернуть, перезапуск, /export'
    - Один scoped golangci-lint run по изменённым пакетам перед коммитом
created_at: "2026-09-16T08:00:30.677241Z"
updated_at: "2026-09-16T08:00:30.677241Z"
---

## Body

**Родитель:** context-window-management (эпик). Зависит от aside-entry-and-engine-ask.

**Что.** `ItemAside` в `session.Project`; виджет `block.AsideBlock`: заголовок — вопрос и «re: <превью якоря>», когда якорь не текущий лист; тело — ответ (markdown как у assistant). Новый токен темы `Aside` в светлой и тёмной темах — отдельный цвет полосы/заголовка, чтобы aside читался как побочная ветка. Сворачивается кликом по заголовку (`HoverTitleRows`, `OnToggle` в `Mapper`, состояние хранится как у tool-блоков), тултип «свернуть/развернуть», развёрнут по умолчанию. Replay ставит строку на её место в ленте (после листа на момент вопроса); `TranscriptPane` понимает `AsideUpdate` как tail-патч на стриме. Экспорт в markdown (`ExportSession`) помечает aside как побочный вопрос.

**Вне скоупа:** режим композера, кнопки в сообщениях.

## Acceptance Criteria

- Aside показывается отдельным цветом в обеих темах, с вопросом в заголовке и ссылкой на якорь, когда он не текущий лист
- Клик по заголовку сворачивает/разворачивает, hover подсвечивает заголовок, dwell показывает тултип
- Стрим ответа обновляет строку через tail-патч; после replay строка на том же месте и в том же состоянии по умолчанию
- Экспорт в markdown содержит aside с пометкой
- Строка в CHANGELOG под Unreleased

## Verification Plan

1. go test ./internal/session/... ./internal/components/block/... ./internal/tui/transcript/... ./internal/components/...
2. Ручная проверка в обеих темах: /btw, свернуть, развернуть, перезапуск, /export
3. Один scoped golangci-lint run по изменённым пакетам перед коммитом

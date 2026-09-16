---
id: aside-composer-mode
title: 'Режим btw в композере: кнопка-lead, хоткей Ctrl+T, подсказка'
status: todo
priority: high
model_level: medium
task_type: feature
parent_id: context-window-management
tags:
    - tui
    - composer
    - aside
    - keys
branch: feature/aside-composer-mode
worktree_path: .worktrees/aside-composer-mode
acceptance_criteria:
    - Lead ⏵⏵ btw виден в режиме, с якорем показывает @<id>; клик по lead включает/выключает режим; hover и тултип работают
    - Ctrl+T включает режим (переназначаемо через таблицу keys), help и палитра знают команду
    - Enter в режиме отправляет aside и выключает режим; Esc выходит, текст остаётся; в голосовом режиме и с ! режим не включается
    - Кнопка ? btw в сообщении включает режим с якорем этого сообщения
    - Строка в CHANGELOG под Unreleased
verification_plan:
    - go test ./internal/tui/composer/... ./internal/components/chat/... ./internal/tui/keys/... ./internal/tui/submit/...
    - 'Ручная проверка: Ctrl+T → вопрос → ответ-aside; кнопка btw на старом ответе → lead с @id → вопрос по тому контексту'
    - Один scoped golangci-lint run по изменённым пакетам перед коммитом
created_at: "2026-09-16T08:00:30.678223Z"
updated_at: "2026-09-16T08:00:30.678223Z"
---

## Body

**Родитель:** context-window-management (эпик). Зависит от transcript-message-actions и aside-entry-and-engine-ask.

**Что.** Режим композера «aside» по образцу голосового режима (internal/tui/composer/voice.go) и posture lead (`applyPosture`): lead `⏵⏵ btw` (с якорем — `⏵⏵ btw @<id>`) цветом `Aside`, плейсхолдер «побочный вопрос — ответ не попадёт в контекст». Включается: кнопкой `? btw` в сообщении (`View.AsideAbout(id)` ставит якорь и включает режим), кликом по lead (lead становится click-addressable: hover-тинт, рука, тултип «задать побочный вопрос — ответ останется вне контекста (Ctrl+T)»), хоткеем `CmdAside` (умолчание `Ctrl+T`, таблица `internal/tui/keys/table.go`, профили, строка в help), `/btw` без аргументов, пунктом палитры Ctrl+K. Enter отправляет текст как aside (Engine.Aside через Submitter) и выключает режим (one-shot); Esc выходит из режима, сохраняя текст. Взаимоисключение с голосовым режимом и `!`-префиксом по образцу voice.go.

**Вне скоупа:** блок ответа (aside-transcript-row), кнопки в сообщениях.

## Acceptance Criteria

- Lead ⏵⏵ btw виден в режиме, с якорем показывает @<id>; клик по lead включает/выключает режим; hover и тултип работают
- Ctrl+T включает режим (переназначаемо через таблицу keys), help и палитра знают команду
- Enter в режиме отправляет aside и выключает режим; Esc выходит, текст остаётся; в голосовом режиме и с ! режим не включается
- Кнопка ? btw в сообщении включает режим с якорем этого сообщения
- Строка в CHANGELOG под Unreleased

## Verification Plan

1. go test ./internal/tui/composer/... ./internal/components/chat/... ./internal/tui/keys/... ./internal/tui/submit/...
2. Ручная проверка: Ctrl+T → вопрос → ответ-aside; кнопка btw на старом ответе → lead с @id → вопрос по тому контексту
3. Один scoped golangci-lint run по изменённым пакетам перед коммитом

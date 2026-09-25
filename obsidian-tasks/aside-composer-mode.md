---
id: aside-composer-mode
title: 'Режим btw в композере: кнопка-lead, хоткей Ctrl+T, подсказка'
status: done
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
updated_at: "2026-09-25T09:00:00.000000Z"
---

## Body

**Родитель:** context-window-management (эпик). Зависит от transcript-message-actions и aside-entry-and-engine-ask.

**Что.** Режим композера «aside» по образцу голосового режима (internal/tui/composer/voice.go) и posture lead (`applyPosture`): lead `⏵⏵ btw` (с якорем — `⏵⏵ btw @<id>`) цветом `Aside`, плейсхолдер «побочный вопрос — ответ не попадёт в контекст». Включается: кнопкой `? btw` в сообщении (`View.AsideAbout(id)` ставит якорь и включает режим), кликом по lead (lead становится click-addressable: hover-тинт, рука, тултип «задать побочный вопрос — ответ останется вне контекста (Ctrl+T)»), хоткеем `CmdAside` (умолчание `Ctrl+T`, таблица `internal/tui/keys/table.go`, профили, строка в help), `/btw` без аргументов, пунктом палитры Ctrl+K. Enter отправляет текст как aside (Engine.Aside через Submitter) и выключает режим (one-shot); Esc выходит из режима, сохраняя текст. Взаимоисключение с голосовым режимом и `!`-префиксом по образцу voice.go.

**Вне скоупа:** блок ответа (aside-transcript-row), кнопки в сообщениях.

**Started (2026-09-24).** Утверждён дизайн: BTW из сообщения активирует одноразовый режим композера с якорем; вопрос и ответ не должны попадать в следующие запросы модели.

**Note (2026-09-24).** Worktree is active on feature/aside-composer-mode. Anchored btw button, one-shot composer, lead/shortcut/palette and bare /btw are implemented with test-first regressions; focused Go tests, scoped format/lint and review in progress. The existing /btw <question> path is preserved.

**Note (2026-09-24).** Scoped lint was run once on the changed packages; it reported two findings: Handle cyclomatic complexity 76>73 and strings.Index in the new lead test. Both were changed (mode key handler extracted, strings.Cut used), without rerunning lint per the one-run rule. Formatting, changed-package build/tests and targeted integration tests are the verification path; manual TUI smoke test remains for PR review.

**Note (2026-09-24).** Проверено: форматирование изменённых Go-файлов, git diff --check, go build для пяти изменённых пакетов и go test для шести затронутых/связанных пакетов — успешно. Интеграционный тест проверяет якорь и отсутствие вопроса/ответа в следующем запросе. Первоначальный единственный прогон scoped lint выявил два замечания; оба исправлены, повторный прогон не делался по правилу одного запуска. Ручная проверка TUI остаётся на этапе PR.

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

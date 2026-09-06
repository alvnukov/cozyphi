---
id: developer-mode-agents
title: 12 — Показать метаданные агентов, заданий и watches
status: blocked
priority: medium
model_level: medium
task_type: feature
parent_id: developer-mode-readonly
tags:
    - developer-mode
acceptance_criteria:
    - agents показывает включённость, role pins/inheritance, доступные ограничения и безопасные состояния заданий текущей сессии.
    - diagnostics.watches показывает поддержку, лимиты, число/состояния наблюдений своей сессии; headless явно unavailable/not_applicable.
    - Developer-доступ детей не наследуется; snapshot не расширяет доступ к заданиям других сессий и не раскрывает prompt/result/transcript.
    - Не выводятся shell commands, labels с произвольным чувствительным текстом, raw output и errors; только allowlisted metadata.
    - Чтение не запускает/останавливает задачи или watches, не делает recovery/повторный spawn и не ждёт завершения job.
verification_plan:
    - Fixtures role pin/inherit/stale pin, job states и watch limits; headless без watch manager.
    - 'Две сессии и child: snapshot показывает только разрешённый scope; нет чужих prompts/results.'
    - Spies spawn/cancel/recover/watch-start/stop/wait не вызываются; sentinel commands/labels/errors redacted.
created_at: "2026-09-06T09:08:35.746978Z"
updated_at: "2026-09-06T09:08:35.746978Z"
---

## Body

**Что построить.** Сквозное наблюдение execution metadata агентов/заданий и watches. Читать epic developer-mode-readonly.

**Blocked by:** developer-mode-headless.

**Scope.** Agents-категория для roles/jobs, diagnostics.watches для watches; не дублировать одно состояние в двух категориях. Использовать bound owner/session identity, а не обход всего process manager. Process-wide limits можно показывать агрегатами без чужого содержимого. Role model source разрешается существующим resolver, а не новым выбором модели. Parent developer flag не является child capability.

**Работа.** Task worktree; профильные fixtures без процессов/моделей; closeout по epic.

## Acceptance Criteria

- agents показывает включённость, role pins/inheritance, доступные ограничения и безопасные состояния заданий текущей сессии.
- diagnostics.watches показывает поддержку, лимиты, число/состояния наблюдений своей сессии; headless явно unavailable/not_applicable.
- Developer-доступ детей не наследуется; snapshot не расширяет доступ к заданиям других сессий и не раскрывает prompt/result/transcript.
- Не выводятся shell commands, labels с произвольным чувствительным текстом, raw output и errors; только allowlisted metadata.
- Чтение не запускает/останавливает задачи или watches, не делает recovery/повторный spawn и не ждёт завершения job.

## Verification Plan

1. Fixtures role pin/inherit/stale pin, job states и watch limits; headless без watch manager.
2. Две сессии и child: snapshot показывает только разрешённый scope; нет чужих prompts/results.
3. Spies spawn/cancel/recover/watch-start/stop/wait не вызываются; sentinel commands/labels/errors redacted.

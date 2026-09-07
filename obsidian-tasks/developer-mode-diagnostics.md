---
id: developer-mode-diagnostics
title: 15 — Показать диагностику и ограничить snapshot по времени и размеру
status: done
priority: medium
model_level: medium
task_type: feature
parent_id: developer-mode-readonly
tags:
    - developer-mode
acceptance_criteria:
    - diagnostics показывает configured/effective logging, telemetry и включённость pprof, без чтения логов/профилей или обращения к endpoint.
    - Explain показывает только известные источники и restart semantics; наличие существующего pprof ENV не включает developer mode.
    - Для каждого owner фиксируются observed_at/revision/freshness; общий snapshot явно partial и не объявлен глобально атомарным.
    - Заданы именованные лимиты размера/времени; cancellation и закрытие owner/process завершают ожидание без goroutine leaks и глобального lock.
    - Audit содержит только безопасные action/category/scope/result/partial metadata, без payload, credentials и raw errors; остальные collectors используют общий путь.
verification_plan:
    - 'Fake owners ready/unavailable/closed/canceled: частичные результаты, deadlines и корректные revisions.'
    - Регрессии repeated snapshot/shutdown и goroutine lifecycle; scoped race tests.
    - Sentinel logs/profiles/error payload отсутствуют во всех выходах; spy network/read-profile подтверждает отсутствие обращения.
created_at: "2026-09-06T09:09:20.78278Z"
updated_at: "2026-09-07T01:18:10.235165Z"
---

## Body

**Что построить.** Реальное состояние диагностических механизмов плюс завершение общего bounded snapshot/audit поведения. Читать epic developer-mode-readonly.

**Blocked by:** developer-mode-headless.

**Scope.** Logging/telemetry/pprof metadata и общие гарантии агрегатора. Watch-раздел делает developer-mode-agents и не блокирует эту задачу. Уточнить базовые лимиты из 01, не создавать второй executor или scheduler. Cancellation должна завершать реальную работу, не просто оставлять зависшую goroutine за timeout. Предпочитать локальные detached snapshots владельцев и context-aware bounded reads. Не добавлять новые логирование сырых данных или dump возможности.

**Работа.** Task worktree; fake owners/clock где доступно, никаких настоящих pprof запросов; узкие tests и closeout по epic.

## Acceptance Criteria

- diagnostics показывает configured/effective logging, telemetry и включённость pprof, без чтения логов/профилей или обращения к endpoint.
- Explain показывает только известные источники и restart semantics; наличие существующего pprof ENV не включает developer mode.
- Для каждого owner фиксируются observed_at/revision/freshness; общий snapshot явно partial и не объявлен глобально атомарным.
- Заданы именованные лимиты размера/времени; cancellation и закрытие owner/process завершают ожидание без goroutine leaks и глобального lock.
- Audit содержит только безопасные action/category/scope/result/partial metadata, без payload, credentials и raw errors; остальные collectors используют общий путь.

## Verification Plan

1. Fake owners ready/unavailable/closed/canceled: частичные результаты, deadlines и корректные revisions.
2. Регрессии repeated snapshot/shutdown и goroutine lifecycle; scoped race tests.
3. Sentinel logs/profiles/error payload отсутствуют во всех выходах; spy network/read-profile подтверждает отсутствие обращения.

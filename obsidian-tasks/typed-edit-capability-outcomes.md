---
id: typed-edit-capability-outcomes
title: Типизированные исходы edit capability
status: todo
priority: high
model_level: medium
task_type: refactor
parent_id: reliable-model-file-edits
acceptance_criteria:
    - Ledger или замещающий глубокий модуль возвращает типизированные причины отказа без string matching внутри writetool
    - Различаются как минимум no_snapshot, snapshot_consumed, anchor_not_observed, mixed_grants, snapshot_evicted и tag_changed
    - Каждый отказ edit содержит стабильный код, фразу Do not retry the same call unchanged и ровно один конкретный recovery-шаг
    - Существующие guarantees claim/release, одноразовость grants и fail-closed поведение сохранены
    - Analyzer распознаёт стабильные коды и продолжает классифицировать старые текстовые ошибки
verification_plan:
    - Добавить table-driven тесты публичного интерфейса модуля для каждого outcome
    - Запустить go test для editledger, readtool и writetool
    - Запустить analyzer на сохранённом корпусе и проверить unknown=0
    - Проверить, что сообщения не содержат tool args или содержимое replacement
created_at: "2026-09-04T22:21:16.730658Z"
updated_at: "2026-09-04T22:21:16.730658Z"
---

## Body

Ввести один типизированный результат разрешения edit capability и провести его от provenance ledger до model-facing результата edit. Это prefactor для последующих автоматических исправлений и самостоятельное устранение слепых ретраев.

Интерфейс модуля должен скрывать карты grants и правила их потребления; callers не должны самостоятельно восстанавливать причину отказа. Ошибка содержит code/retryability/recovery, а строковое представление формируется на границе инструмента без путей к секретам и содержимого правок.

**Blocked by:** None — can start immediately.

**Why first:** текущая одна фраза current-session editable read объединяет несколько разных состояний и не даёт модели выбрать правильное восстановление.

## Acceptance Criteria

- Ledger или замещающий глубокий модуль возвращает типизированные причины отказа без string matching внутри writetool
- Различаются как минимум no_snapshot, snapshot_consumed, anchor_not_observed, mixed_grants, snapshot_evicted и tag_changed
- Каждый отказ edit содержит стабильный код, фразу Do not retry the same call unchanged и ровно один конкретный recovery-шаг
- Существующие guarantees claim/release, одноразовость grants и fail-closed поведение сохранены
- Analyzer распознаёт стабильные коды и продолжает классифицировать старые текстовые ошибки

## Verification Plan

1. Добавить table-driven тесты публичного интерфейса модуля для каждого outcome
2. Запустить go test для editledger, readtool и writetool
3. Запустить analyzer на сохранённом корпусе и проверить unknown=0
4. Проверить, что сообщения не содержат tool args или содержимое replacement

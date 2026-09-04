---
id: typed-edit-capability-outcomes
title: Типизированные исходы edit capability
status: done
priority: high
model_level: medium
task_type: refactor
parent_id: reliable-model-file-edits
branch: refactor/typed-edit-capability-outcomes
worktree_path: .worktrees/typed-edit-capability-outcomes
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
updated_at: "2026-09-04T23:53:41.757754Z"
---

## Body

Ввести один типизированный результат разрешения edit capability и провести его от provenance ledger до model-facing результата edit. Это prefactor для последующих автоматических исправлений и самостоятельное устранение слепых ретраев.

Интерфейс модуля должен скрывать карты grants и правила их потребления; callers не должны самостоятельно восстанавливать причину отказа. Ошибка содержит code/retryability/recovery, а строковое представление формируется на границе инструмента без путей к секретам и содержимого правок.

**Blocked by:** review-model-edit-reliability-design — реализация следует утверждённым контрактам capability lifecycle и typed recovery.

**Why first after design:** текущая одна фраза current-session editable read объединяет несколько разных состояний и не даёт модели выбрать правильное восстановление.

**Started (2026-09-05).** 2026-09-05 — старт по утверждённому плану (rev 29). Worktree .worktrees/typed-edit-capability-outcomes, ветка refactor/typed-edit-capability-outcomes, база main 0634448.

**Done (2026-09-05).** Landed: commit 8492dec (refactor(tools): give edit refusals stable typed outcome codes) merged to main as eb147a0 (--no-ff, CHANGELOG Unresolved-conflict resolved by combining entries). Scope: internal/tools/editledger (typed Outcome + Code()/Refused(), Claim/Release/Authorize, ring 8, eviction 16), internal/tools/writetool (EditRefusal{Code,What,Next} rendering `[edit:<code>] … Do not retry the same call unchanged. …`, 11 codes incl. mixed_grants/snapshot_consumed/tag_changed; ApplyHashlineEdit (string,int,error) with duplicate-drop count in success body; anchors parsed before Claim, Release on error), scripts/analyze_edit_errors.py (STABLE_EDIT_CODE classify-ahead-of-legacy; A/B vs HEAD identical, unknown=0 on 233 transcripts). Gates scoped to editledger/writetool/readtool: test, vet, golangci-lint — green. Baseline targets (−70% stale+no_capability, −80% blind retries) are for verify-model-edit-reliability to measure post-rollout.

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

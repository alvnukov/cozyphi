---
id: plan-v2-atomic-batch-patch
title: Add atomic domain-specific Plan patch operations
status: done
priority: high
model_level: low
task_type: feature
parent_id: plan-v2-agent-work-contract
tags:
    - plan-v2
    - tool-api
    - atomicity
    - revision
    - tdd
acceptance_criteria:
    - Один patch атомарно меняет несколько plan/step fields и структуру по stable IDs.
    - Ошибка любой операции оставляет plan и revision без частичных изменений.
    - Stale expected_revision возвращает actual revision и компактный changed summary.
    - Patch не позволяет напрямую менять status, approval, evidence history или audit metadata.
    - Validation errors указывают index операции, step ID и проблемное поле.
verification_plan:
    - Показать red→green tests multi-op atomicity, rollback и revision conflict.
    - Запустить focused plantool/session tests с race detector.
    - Проверить, что обычный patch response содержит delta, а не полный snapshot.
created_at: "2026-08-28T10:51:18.606782Z"
updated_at: "2026-08-28T13:59:45.307433Z"
---

## Body

Blocked by: plan-v2-create-get-actions. Перевести в todo только после blocker=done.

Добавить action=patch с expected_revision и transaction-like operations по stable IDs: set plan fields, update_step, insert_before/after, remove pending step, reorder IDs, add/update/remove constraint or success criterion, replace_context. Не использовать JSON Pointer и array indexes. Семантика scalar patch: absent=unchanged, value=replace, null=clear only optional. working_context заменяется целиком, blind append отсутствует. Весь batch применим all-or-none.

TDD seam: plan tool patch contract + persisted result. Сначала red tests на multi-op success, stale revision и rollback при ошибке средней операции.

## Acceptance Criteria

- Один patch атомарно меняет несколько plan/step fields и структуру по stable IDs.
- Ошибка любой операции оставляет plan и revision без частичных изменений.
- Stale expected_revision возвращает actual revision и компактный changed summary.
- Patch не позволяет напрямую менять status, approval, evidence history или audit metadata.
- Validation errors указывают index операции, step ID и проблемное поле.

## Verification Plan

1. Показать red→green tests multi-op atomicity, rollback и revision conflict.
2. Запустить focused plantool/session tests с race detector.
3. Проверить, что обычный patch response содержит delta, а не полный snapshot.

---
id: plan-v2-auto-finish-archive
title: Auto-finish and archive terminal Plans without stale prompt cost
status: done
priority: medium
model_level: low
task_type: feature
parent_id: plan-v2-agent-work-contract
tags:
    - plan-v2
    - lifecycle
    - archive
    - tokens
    - tdd
acceptance_criteria:
    - Последний valid complete с plan_result автоматически делает plan completed/archived.
    - Finished plan не остаётся active gate state и не инжектируется полным snapshot на следующих inference.
    - Unmet success criterion, blocker или required cancelled step запрещает auto-finish с actionable error.
    - Archived plan доступен full get/history и может быть явно reopened с reason.
    - Новый plan создаётся без stale fields предыдущего, а revision/audit history остаётся доступной.
verification_plan:
    - Показать red→green lifecycle tests terminal, blocked и reopen cases.
    - Запустить session/plantool/prompt tests с resume round-trip.
    - Измерить post-completion prompt cost и закрепить bounded result.
created_at: "2026-08-28T10:51:18.612622Z"
updated_at: "2026-08-28T23:31:09.967475Z"
---

## Body

Blocked by: plan-v2-step-transition-state-machine, plan-v2-compact-prompt-projection. Перевести в todo после обоих blockers.

Когда последний обязательный step завершён и нет blockers/cancelled required work, complete с plan_result переводит plan в completed и архивирует полный audit без отдельного archive call. Active prompt перестаёт содержать полный finished plan и оставляет bounded final result до следующего plan. Explicit reopen восстанавливает archived plan новой revision. Нельзя auto-finish при unmet success criterion.

TDD seam: public plan lifecycle + prompt after terminal transition.

## Acceptance Criteria

- Последний valid complete с plan_result автоматически делает plan completed/archived.
- Finished plan не остаётся active gate state и не инжектируется полным snapshot на следующих inference.
- Unmet success criterion, blocker или required cancelled step запрещает auto-finish с actionable error.
- Archived plan доступен full get/history и может быть явно reopened с reason.
- Новый plan создаётся без stale fields предыдущего, а revision/audit history остаётся доступной.

## Verification Plan

1. Показать red→green lifecycle tests terminal, blocked и reopen cases.
2. Запустить session/plantool/prompt tests с resume round-trip.
3. Измерить post-completion prompt cost и закрепить bounded result.

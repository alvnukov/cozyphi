---
id: plan-v2-jit-risk-approval
title: Require just-in-time approval for irreversible Plan steps
status: done
priority: high
model_level: low
task_type: feature
parent_id: plan-v2-agent-work-contract
tags:
    - plan-v2
    - approval
    - risk
    - security
    - tdd
acceptance_criteria:
    - JIT-marked step блокируется после общего approval до отдельного user approval.
    - JIT token/flag scoped to exact plan revision and stable step ID.
    - Material contract change, reopen с изменённым action или другой step инвалидируют старое JIT approval.
    - Non-JIT steps не получают дополнительного model/API round.
    - Denial содержит human-readable action/risk без утечки model context или tool secrets.
verification_plan:
    - Показать red→green plangate/controller tests approve, deny, stale revision и cross-step reuse.
    - Запустить approval/executor tests с race detector.
    - Проверить representative push/tag/deploy/delete/send policy fixtures.
created_at: "2026-08-28T10:51:18.613572Z"
updated_at: "2026-08-28T18:07:55.392633Z"
---

## Body

Blocked by: plan-v2-field-aware-approval, plan-v2-auto-start-stable-step. Перевести в todo после обоих blockers.

Реализовать optional step risk/approval=just_in_time. Общего approval плана недостаточно для помеченных irreversible external effects (push/tag/deploy/delete/send). Gate останавливает вызов и выдаёт человеку точный step/action/risk diff. Разрешение user-owned, привязано к plan revision и stable step ID, не переносится на material change или другой step. Отказ/истечение не меняют step на completed.

TDD seam: plangate approval decision and user approval handoff.

## Acceptance Criteria

- JIT-marked step блокируется после общего approval до отдельного user approval.
- JIT token/flag scoped to exact plan revision and stable step ID.
- Material contract change, reopen с изменённым action или другой step инвалидируют старое JIT approval.
- Non-JIT steps не получают дополнительного model/API round.
- Denial содержит human-readable action/risk без утечки model context или tool secrets.

## Verification Plan

1. Показать red→green plangate/controller tests approve, deny, stale revision и cross-step reuse.
2. Запустить approval/executor tests с race detector.
3. Проверить representative push/tag/deploy/delete/send policy fixtures.

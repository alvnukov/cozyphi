---
id: plan-v2-tool-attempt-evidence
title: Attach gated tool attempts and evidence refs to Plan steps
status: done
priority: high
model_level: low
task_type: feature
parent_id: plan-v2-agent-work-contract
tags:
    - plan-v2
    - evidence
    - audit
    - executor
    - tdd
acceptance_criteria:
    - Accepted tool call создаёт ровно один durable attempt у правильного stable step ID.
    - Success, runtime failure, cancellation и lost result имеют различимые bounded statuses.
    - complete rejects foreign/missing evidence ref and accepts a matching persisted attempt.
    - Repeated mutation/tool result reconciliation не создаёт duplicate attempts.
    - Compact plan view не содержит raw tool output, но full audit позволяет найти исходный call.
verification_plan:
    - Показать red→green executor-to-plan attempt tests.
    - Запустить focused executor/session tests с race detector и cancellation cases.
    - Проверить serialized size на bounded output и отсутствие raw secret fixture.
created_at: "2026-08-28T10:51:18.609841Z"
updated_at: "2026-08-28T16:18:04.1511Z"
---

## Body

Blocked by: plan-v2-step-transition-state-machine, plan-v2-auto-start-stable-step. Перевести в todo после обоих blockers.

Каждый gateable tool call с plan_step автоматически создаёт bounded attempt record: call ID, tool, timestamps/status и compact result summary/ref. Полный output не дублируется в plan. complete может ссылаться на существующие successful attempts. Evidence history append-only; supersede требует reason и сохраняет старую запись в audit. Ошибочная попытка не переводит логический шаг в failed/completed.

TDD seam: executor result → durable plan attempt/evidence public readback.

## Acceptance Criteria

- Accepted tool call создаёт ровно один durable attempt у правильного stable step ID.
- Success, runtime failure, cancellation и lost result имеют различимые bounded statuses.
- complete rejects foreign/missing evidence ref and accepts a matching persisted attempt.
- Repeated mutation/tool result reconciliation не создаёт duplicate attempts.
- Compact plan view не содержит raw tool output, но full audit позволяет найти исходный call.

## Verification Plan

1. Показать red→green executor-to-plan attempt tests.
2. Запустить focused executor/session tests с race detector и cancellation cases.
3. Проверить serialized size на bounded output и отсутствие raw secret fixture.

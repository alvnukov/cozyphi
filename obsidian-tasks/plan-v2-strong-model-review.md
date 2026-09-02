---
id: plan-v2-strong-model-review
title: Full Plan v2 Standards and Spec review by a very-high model
status: blocked
priority: high
model_level: very_high
task_type: test
parent_id: plan-v2-agent-work-contract
tags:
    - plan-v2
    - review
    - quality-gate
    - very-high-model
acceptance_criteria:
    - Review охватывает полный feature diff и отдельно отчёт по Standards и Spec.
    - Каждое epic acceptance criterion сопоставлено к коду, тесту и наблюдаемому evidence.
    - Проверены state transitions, revision conflicts, idempotency, races, approval/JIT boundaries и failure atomicity.
    - Проверены prompt budget, API-round measurement, injection/redaction fixtures и legacy resume compatibility.
    - Все findings заведены child tasks, исправлены и перепроверены полным rerun; unresolved findings = 0.
    - Review report явно разрешает закрытие epic; до этого plan-v2-agent-work-contract остаётся blocked.
verification_plan:
    - Запустить/проверить focused, integration, race, fmt, lint и full test gates на финальном commit set.
    - Сравнить v1/v2 tool-call trace и prompt-byte measurements с epic targets.
    - Провести adversarial review malformed operations, stale revisions, replay, secret output и JIT reuse.
    - После любых follow-up fixes повторить полный Standards+Spec review, а не только diff исправления.
created_at: "2026-08-28T10:51:18.618874Z"
updated_at: "2026-08-28T10:51:18.618874Z"
---

## Body

Blocked by: plan-v2-contract-migration-integration and every implementation/follow-up child of plan-v2-agent-work-contract. Эта задача выполняется ТОЛЬКО моделью уровня very_high после зелёного integration task.

Провести полное read-only Standards+Spec review всей фичи от base до integration commit, а не выборочную проверку последнего diff. Проверить domain model, tool API ergonomics для слабой модели, state-machine invariants, atomicity/idempotency/concurrency, approval/JIT safety, prompt injection/redaction, persistence/resume/migration, compact token projection, sidebar human control, observability и фактическое сокращение API rounds. Запустить или подтвердить полную verification matrix.

Если найден любой actionable finding: создать отдельный child follow-up под epic (model_level low, если решение уже определено; иначе appropriate higher level), оставить review и epic blocked, дождаться исправления и повторить ПОЛНЫЙ review. Review можно перевести в done только при нуле unresolved findings. Epic можно закрыть только после done этой задачи и всех созданных review follow-ups.

## Acceptance Criteria

- Review охватывает полный feature diff и отдельно отчёт по Standards и Spec.
- Каждое epic acceptance criterion сопоставлено к коду, тесту и наблюдаемому evidence.
- Проверены state transitions, revision conflicts, idempotency, races, approval/JIT boundaries и failure atomicity.
- Проверены prompt budget, API-round measurement, injection/redaction fixtures и legacy resume compatibility.
- Все findings заведены child tasks, исправлены и перепроверены полным rerun; unresolved findings = 0.
- Review report явно разрешает закрытие epic; до этого plan-v2-agent-work-contract остаётся blocked.

## Verification Plan

1. Запустить/проверить focused, integration, race, fmt, lint и full test gates на финальном commit set.
2. Сравнить v1/v2 tool-call trace и prompt-byte measurements с epic targets.
3. Провести adversarial review malformed operations, stale revisions, replay, secret output и JIT reuse.
4. После любых follow-up fixes повторить полный Standards+Spec review, а не только diff исправления.

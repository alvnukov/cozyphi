---
id: plan-v2-contract-migration-integration
title: Migrate Plan callers and prove the end-to-end v2 workflow
status: blocked
priority: high
model_level: low
task_type: test
parent_id: plan-v2-agent-work-contract
tags:
    - plan-v2
    - migration
    - integration
    - compatibility
    - tdd
acceptance_criteria:
    - Новые prompts и internal callers используют stable step IDs и operation API, не full snapshot replace.
    - Legacy persisted session и replayed legacy tool call имеют deterministic compatibility behavior.
    - Happy path требует один create, ноль plan-only calls между working steps и один terminal completion.
    - Material edit, JIT step, stale revision, tool failure, compaction/resume и restart проходят end-to-end fixtures.
    - Focused tests, race tests и repository fmt/lint/test gates green.
verification_plan:
    - Показать red end-to-end fixture до caller migration и green после minimal migration.
    - Запустить plan/session/executor/plangate/TUI integration suites с race detector.
    - Запустить repository fmt-check, lint и test gates.
    - Сохранить measured API-round and prompt-byte comparison v1 vs v2 в task evidence.
created_at: "2026-08-28T10:51:18.617927Z"
updated_at: "2026-08-28T10:51:18.617927Z"
---

## Body

Blocked by: plan-v2-piggyback-transition, plan-v2-compact-prompt-projection, plan-v2-auto-finish-archive, plan-v2-jit-risk-approval, plan-v2-sidebar-human-control, plan-v2-context-and-evidence-safety, plan-v2-observability-budget. Перевести в todo после всех blockers.

Contract-фаза expand–contract: переключить internal callers, prompt guidance и tests на stable IDs и action API. Legacy numeric plan_step и steps-only snapshot остаются только в явно документированном compatibility adapter для resumed sessions; новые prompts их не рекомендуют. Добавить end-to-end eval: create→user approval→working call auto-start→piggyback complete/context/next call→evidence→final complete/archive, плюс material reapproval, JIT denial и recovery после restart. Не удалять legacy decode, пока fixture старой session не проходит.

TDD seam: black-box engine/session/tool workflow.

## Acceptance Criteria

- Новые prompts и internal callers используют stable step IDs и operation API, не full snapshot replace.
- Legacy persisted session и replayed legacy tool call имеют deterministic compatibility behavior.
- Happy path требует один create, ноль plan-only calls между working steps и один terminal completion.
- Material edit, JIT step, stale revision, tool failure, compaction/resume и restart проходят end-to-end fixtures.
- Focused tests, race tests и repository fmt/lint/test gates green.

## Verification Plan

1. Показать red end-to-end fixture до caller migration и green после minimal migration.
2. Запустить plan/session/executor/plangate/TUI integration suites с race detector.
3. Запустить repository fmt-check, lint и test gates.
4. Сохранить measured API-round and prompt-byte comparison v1 vs v2 в task evidence.

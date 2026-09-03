---
id: plan-v2-sidebar-human-control
title: Render Plan v2 progress, rationale, diffs and approvals in the sidebar
status: done
priority: medium
model_level: low
task_type: feature
parent_id: plan-v2-agent-work-contract
tags:
    - plan-v2
    - tui
    - ux
    - approval
    - tdd
acceptance_criteria:
    - Default sidebar остаётся кратким и показывает goal, progress, active step и blockers.
    - Expanded view показывает rationale/criteria/context/evidence без raw tool logs.
    - Checkbox/icon state всегда выводится из canonical status и не может расходиться с ним.
    - Reapproval и JIT UI показывают bounded material diff и точный step.
    - Archive/reopen/resume и narrow terminal rendering не теряют состояние и не panic.
verification_plan:
    - Показать red→green view-model/render tests для каждого status и approval state.
    - Запустить focused TUI/controller tests и race tests.
    - Запустить responsive golden tests для narrow/default/wide widths и long plan.
created_at: "2026-08-28T10:51:18.615131Z"
updated_at: "2026-08-29T08:37:44.057085Z"
---

## Body

Blocked by: plan-v2-field-aware-approval, plan-v2-compact-prompt-projection, plan-v2-auto-finish-archive, plan-v2-jit-risk-approval. Перевести в todo после всех blockers.

Обновить существующий plan sidebar, не создавать второй widget. Default view показывает goal, progress, active action и blockers; details раскрывают approach, why, done_when, working context, outcomes и evidence refs. Статусы визуализируются из state machine ([ ], active, [x], blocked, cancelled), без отдельного checked state. Material reapproval показывает только diff. JIT prompt показывает точный irreversible step. Long content остаётся independently scrollable и responsive.

TDD seam: controller/view-model/rendering behavior, не private widget internals.

## Acceptance Criteria

- Default sidebar остаётся кратким и показывает goal, progress, active step и blockers.
- Expanded view показывает rationale/criteria/context/evidence без raw tool logs.
- Checkbox/icon state всегда выводится из canonical status и не может расходиться с ним.
- Reapproval и JIT UI показывают bounded material diff и точный step.
- Archive/reopen/resume и narrow terminal rendering не теряют состояние и не panic.

## Verification Plan

1. Показать red→green view-model/render tests для каждого status и approval state.
2. Запустить focused TUI/controller tests и race tests.
3. Запустить responsive golden tests для narrow/default/wide widths и long plan.

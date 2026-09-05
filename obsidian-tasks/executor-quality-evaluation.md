---
id: executor-quality-evaluation
title: Evaluate executor quality across model capability tiers
status: blocked
priority: high
model_level: medium
task_type: test
parent_id: plan-driven-interactive-executors
tags:
    - executors
    - tests
    - evaluation
acceptance_criteria:
    - Scripted invariants pass; failures are recorded without cherry-picking.
    - Bounded high/medium/low trials use comparable tasks and disclose selected models, reasoning effort and context modes separately.
    - False acceptance, lost requirements, rework quality, authority violations and unresolved contradictions take precedence over timing/cost.
    - Passing samples are not presented as general proof of quality; parent/human review conclusions before rollout.
    - Any false acceptance or authority breach blocks rollout and is routed to its owning slice.
verification_plan:
    - Run assign → local plan returned → approved → execute → result returned → reapprove → accepted through real plan/admission/receipt paths.
    - Verify human conflict/stop, restore, budget mismatch and independent counterexample review, including deterministic races.
    - Run bounded capability-tier trials with disclosed model/effort/context and save evidence plus limitations.
    - Have parent/human review the report before rollout; preserve unresolved findings as blockers.
created_at: "2026-09-05T23:23:01.186405Z"
updated_at: "2026-09-05T23:23:01.186405Z"
---

## Body

**Approved slice:** 15 of [breakdown](../specs/plan-driven-executors-tickets.md). Read the full [contract](../specs/plan-driven-executors.md).

**Blocked by:** executor-independent-review, executor-context-budget, executor-assignment-recovery, executor-status-explanations, executor-prompt-skills, executor-review-negative-matrix, executor-budget-regressions, executor-user-walkthrough.

**Capability rationale:** medium — execute an agreed matrix and investigate bounded discrepancies; new architecture/policy choices are escalations.

**Inputs:** accepted slices, public test harnesses and fixed tasks with human-reviewed expected requirements, artifacts and rejection cases.

**Scope/output:** repeatable integration scenarios, bounded high/medium/low model trials and an evidence report. Measure false acceptance, lost requirements, rework, authority violations and contradictions before timing/cost. Quality remains constant across capability levels.

**Escalation:** false acceptance or authority breach creates a blocking issue for the owning slice; unclear expected quality criteria return to the user, not a replacement speed target.

## Acceptance Criteria

- Scripted invariants pass; failures are recorded without cherry-picking.
- Bounded high/medium/low trials use comparable tasks and disclose selected models, reasoning effort and context modes separately.
- False acceptance, lost requirements, rework quality, authority violations and unresolved contradictions take precedence over timing/cost.
- Passing samples are not presented as general proof of quality; parent/human review conclusions before rollout.
- Any false acceptance or authority breach blocks rollout and is routed to its owning slice.

## Verification Plan

1. Run assign → local plan returned → approved → execute → result returned → reapprove → accepted through real plan/admission/receipt paths.
2. Verify human conflict/stop, restore, budget mismatch and independent counterexample review, including deterministic races.
3. Run bounded capability-tier trials with disclosed model/effort/context and save evidence plus limitations.
4. Have parent/human review the report before rollout; preserve unresolved findings as blockers.

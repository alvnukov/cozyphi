---
id: executor-context-budget
title: Resolve executor context overflow with explicit budget choices
status: blocked
priority: medium
model_level: medium
task_type: feature
parent_id: plan-driven-interactive-executors
tags:
    - executors
    - context
    - compaction
acceptance_criteria:
    - Budget accounting reserves system, tools, plan and output capacity.
    - An oversized launch reports required and available budget; there is no silent prompt-only fallback.
    - Retry uses explicit model choice, narrower assignment or authorized child-local compaction, preserving human model-selection control.
    - Parent context stays unchanged; requirements and approval obligations survive allowed compaction or launch fails visibly.
    - An explicit rebase reconciles identities and records the representation change.
verification_plan:
    - Test fitting, exact-boundary and overflowing controlled-model inputs.
    - Test rejected compaction and unavailable model choices.
    - Compare parent state before/after retry; require a valid complete child input or actionable failure.
created_at: "2026-09-05T23:21:01.56153Z"
updated_at: "2026-09-05T23:21:01.56153Z"
---

## Body

**Approved slice:** 07 of [breakdown](../specs/plan-driven-executors-tickets.md). Read [contract](../specs/plan-driven-executors.md), D3–D4.

**Blocked by:** executor-context-snapshot.

**Capability rationale:** medium — budget policy and snapshot seam are fixed; expose a bounded choice-and-retry flow using existing compaction.

**Inputs:** slice 01 cutoff/size metadata and current model ceiling/compaction behavior.

**Scope/output:** report too-large launches and support an explicit retry with a suitable model, narrower assignment or authorized child-local compaction. Record changed representation; use the existing summarizer. Supply deterministic budget fixtures for slice 13.

**Escalation:** inability to preserve required material blocks launch; compactor architecture changes return to the snapshot owner.

## Acceptance Criteria

- Budget accounting reserves system, tools, plan and output capacity.
- An oversized launch reports required and available budget; there is no silent prompt-only fallback.
- Retry uses explicit model choice, narrower assignment or authorized child-local compaction, preserving human model-selection control.
- Parent context stays unchanged; requirements and approval obligations survive allowed compaction or launch fails visibly.
- An explicit rebase reconciles identities and records the representation change.

## Verification Plan

1. Test fitting, exact-boundary and overflowing controlled-model inputs.
2. Test rejected compaction and unavailable model choices.
3. Compare parent state before/after retry; require a valid complete child input or actionable failure.

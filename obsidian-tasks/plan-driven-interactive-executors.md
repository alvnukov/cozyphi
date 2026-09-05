---
id: plan-driven-interactive-executors
title: Plan-driven retained interactive executors
status: todo
priority: high
model_level: high
task_type: epic
tags:
    - goal
    - executors
    - plan
    - interactive-children
acceptance_criteria:
    - Children inherit full effective parent context and a read-only parent plan without transferring approvals, edit anchors, tools or permission authority.
    - A child owns its local execution plan; its parent approves the exact revision before execution and approves substantial replanning again.
    - Results require separate parent acceptance of current contract revisions and artifact-bound evidence; completion alone never completes the parent step.
    - Rework retains the child session with immutable linked attempts; human-stop, intervention, actual-exit ownership and recovery fences hold.
    - All 15 approved slices and the quality evaluation are accepted; capability tiers preserve the same quality bar.
verification_plan:
    - Use the existing Controller/interactive runtime with real plan, admission and receipt paths and controlled runner/provider.
    - Verify authority, revision, artifact, human-intervention, rework, context-budget and recovery rejection scenarios deterministically.
    - Complete the approved capability-tier trials with disclosed evidence, limitations and human-reviewed conclusions before rollout.
created_at: "2026-09-05T23:19:13.985104Z"
updated_at: "2026-09-05T23:19:13.985104Z"
---

## Body

**Approved scope:** The user approved the 15-slice breakdown and Controller/interactive-runtime testing seam. This is a future implementation epic; publication does not start runtime work.

**Design:** [Standalone contract](../specs/plan-driven-executors.md). **Implementation map:** [Approved numbered breakdown](../specs/plan-driven-executors-tickets.md). Read the contract before implementing any child task.

**Outcome:** assign → propose local plan → parent return/approve → execute → present → parent accept/rework. Parent approach approval never replaces human permission approval. Reuse Plan v2, Controller queue, Manager admission, retained Views and existing compaction; separate context ownership and no nested delegation or second scheduler.

**Capability allocation:** 5 high, 6 medium, 4 low implementation tasks. model_level is capability, not reasoning effort. Foundations settle ambiguous interfaces before bounded medium/low work; every task has mandatory tests and escalation.

**Children, in approved order:** executor-context-snapshot; executor-local-plan-review; executor-result-acceptance; executor-linked-rework; executor-human-authority; executor-independent-review; executor-context-budget; executor-assignment-recovery; executor-review-ui; executor-status-explanations; executor-prompt-skills; executor-review-negative-matrix; executor-budget-regressions; executor-user-walkthrough; executor-quality-evaluation.

**Existing gates:** plan-v2-strong-model-review and plan-v2-contract-migration-integration gate local-plan review; multisession-lifecycle-restore and job-recovery-process-ownership gate recovery. Broader background-attention/close/restore work retains its existing owner. Children carry exact blocking edges; only a task with completed prerequisites may start. Do not treat this container as a ready implementation task.

**Escalation:** authority violations or false acceptance block rollout and create findings for the owning slice; unresolved quality criteria return to the user, never become latency targets.

## Acceptance Criteria

- Children inherit full effective parent context and a read-only parent plan without transferring approvals, edit anchors, tools or permission authority.
- A child owns its local execution plan; its parent approves the exact revision before execution and approves substantial replanning again.
- Results require separate parent acceptance of current contract revisions and artifact-bound evidence; completion alone never completes the parent step.
- Rework retains the child session with immutable linked attempts; human-stop, intervention, actual-exit ownership and recovery fences hold.
- All 15 approved slices and the quality evaluation are accepted; capability tiers preserve the same quality bar.

## Verification Plan

1. Use the existing Controller/interactive runtime with real plan, admission and receipt paths and controlled runner/provider.
2. Verify authority, revision, artifact, human-intervention, rework, context-budget and recovery rejection scenarios deterministically.
3. Complete the approved capability-tier trials with disclosed evidence, limitations and human-reviewed conclusions before rollout.

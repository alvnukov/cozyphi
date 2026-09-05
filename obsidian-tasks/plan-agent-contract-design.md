---
id: plan-agent-contract-design
title: Design plan-driven interactive executors and quality-first acceptance
status: done
priority: high
model_level: high
task_type: design
tags:
    - plan
    - multisession
    - design
branch: design/plan-agent-contract-design
worktree_path: .worktrees/plan-agent-contract-design
acceptance_criteria:
    - Standalone design captures effective-context inheritance, plan ownership, parent-child revision cycles, user intervention and acceptance.
    - Draft dependency graph includes high, medium and low tasks and reuses existing backlog.
    - No runtime implementation or live-session changes.
verification_plan:
    - Check requirements against conversation and current code seams.
    - Check dependency graph for missing or circular blockers and existing-task duplication.
    - Review proposed seams and ticket granularity with user before publishing implementation tickets.
created_at: "2026-09-05T22:56:21.789293Z"
updated_at: "2026-09-05T23:27:59.040696Z"
---

## Body

Design the agreed plan-driven interactive executor workflow. Quality takes priority over latency. Preserve the parent's effective context without transferring authority; retain the child for linked revisions; distinguish task completion from parent acceptance. Produce a standalone spec and proposed vertically sliced backlog, including model_level rationale and escalation. Existing multisession and Plan v2 work must be reused. Ticket breakdown approval precedes publication of implementation tickets.

**Started (2026-09-06).** Approved design/backlog preparation. Direct work only; no runtime implementation. Implementation-ticket publication follows the required breakdown check.

**Note (2026-09-06).** Saved draft specs/plan-driven-executors.md and specs/plan-driven-executors-tickets.md on design/plan-agent-contract-design in .worktrees/plan-agent-contract-design. Includes the newly agreed child-owned local execution plan with parent approval before execution and separate parent result acceptance; user permission remains separate. Proposed 15 slices: 5 high, 6 medium, 4 low, with explicit inputs/output, blockers, acceptance, verification and escalation. Checked local links against canonical main ledger, unique unpublished IDs, all fields and acyclic new-task dependencies; verified existing Plan v2 review/migration, lifecycle restore and process-ownership gates. No runtime code, agents, lint or live sessions touched. Awaiting user review of testing seam and numbered breakdown before creating the epic/implementation tickets; design task remains in progress.

**Done (2026-09-06).** User approved the 15-slice breakdown and Controller/interactive-runtime testing seam before publication. Published epic plan-driven-interactive-executors and all 15 child tasks (5 high, 6 medium, 4 low), each with bounded inputs/output, capability rationale, acceptance, tests, escalation and exact blockers. Only executor-context-snapshot is todo; 14 dependent tasks remain blocked. Standalone contract and approved breakdown landed on main in merge 6cbedc5 (draft b0e5e7d, approval update 3561e5f). Bounded validation passed: unique IDs, capability counts, required fields, exact acyclic dependencies and document links. Reused existing Plan v2 and lifecycle/process gates. No runtime implementation, broad tests/lint, new agents, live-session changes or push. Registry publication is committed separately with this completion note; implementation epic remains open.

## Acceptance Criteria

- Standalone design captures effective-context inheritance, plan ownership, parent-child revision cycles, user intervention and acceptance.
- Draft dependency graph includes high, medium and low tasks and reuses existing backlog.
- No runtime implementation or live-session changes.

## Verification Plan

1. Check requirements against conversation and current code seams.
2. Check dependency graph for missing or circular blockers and existing-task duplication.
3. Review proposed seams and ticket granularity with user before publishing implementation tickets.

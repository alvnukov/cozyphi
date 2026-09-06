---
id: multisession-backlog-refresh
title: Reconcile multisession backlog with cross-project tabs V1
status: in_progress
priority: high
model_level: high
task_type: chore
parent_id: multisession-mode
tags:
    - multisession
    - planning
branch: chore/multisession-backlog-refresh
worktree_path: .worktrees/multisession-backlog-refresh
acceptance_criteria:
    - Existing multisession notes match the approved tab-based cross-project V1 and distinguish source evidence from unverified behavior.
    - Dependencies, remaining acceptance criteria and deferred extensions are consistent; no runtime changes or false implementation completion.
    - Owned ledger changes are checked and committed on main without staging unrelated notes.
verification_plan:
    - Check owned Markdown diff, task metadata/body consistency, local links and prerequisite graph.
    - Verify no runtime files or unrelated notes are staged; commit owned ledger files on main.
created_at: "2026-09-06T13:56:44.183491Z"
updated_at: "2026-09-06T14:03:16.866352Z"
---

## Body

Reconcile the existing multisession-mode epic and its open delivery tasks with the approved tab-based cross-project UX. This is ledger maintenance only, not implementation or a replacement feature backlog. Preserve historical completion evidence, existing assignments and unrelated dirty notes. Link the ambient prompt-cwd defect to refactor-prompt-snapshot without requiring its entire refactor for the first vertical slice.

**Started (2026-09-06).** Approved ledger-only reconciliation. Main-resident task notes only; no runtime worktree or implementation branch is needed. Existing multisession target notes are clean; unrelated main notes remain untouched.

**Note (2026-09-06).** Updated nine existing notes: epic, projects, hotkeys, switch cues, attention, lifecycle, hardening, optional panel and prompt snapshot. Preserved historical child/attention delivery records and all completed child statuses. Narrow prompt context fix belongs to projects; full snapshot refactor remains linked. Deferred panel is blocked on V1 plus renewed UX approval; switch cues and hardening now reflect their real prerequisites. Verification and owned-ledger commit remain.

## Acceptance Criteria

- Existing multisession notes match the approved tab-based cross-project V1 and distinguish source evidence from unverified behavior.
- Dependencies, remaining acceptance criteria and deferred extensions are consistent; no runtime changes or false implementation completion.
- Owned ledger changes are checked and committed on main without staging unrelated notes.

## Verification Plan

1. Check owned Markdown diff, task metadata/body consistency, local links and prerequisite graph.
2. Verify no runtime files or unrelated notes are staged; commit owned ledger files on main.

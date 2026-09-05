---
id: planning-availability-useplan
title: Separate planning availability from useplan enforcement
status: done
priority: high
model_level: high
task_type: feature
tags:
    - planning
branch: feature/planning-availability-useplan
worktree_path: .worktrees/planning-availability-useplan
acceptance_criteria:
    - Planning defaults enabled with build/soft; useplan alone enforces plans; no sidebar planning checkbox.
    - Global enablement is saved and startup-only; execution build/useplan is persisted and applied atomically next round.
    - Soft removes all planning restrictions without bypassing permissions, hooks, read-only roles or input validation; off removes planning surfaces but retains plans.
    - Explicit settings win over legacy/defaults; legacy enabled never implies useplan.
    - Regression tests, documentation and changelog updated; verified changes committed in isolated worktree.
verification_plan:
    - Test persistence/defaults/migration/conflicts/failed saves.
    - Test off/soft/useplan, lifecycle and per-round atomicity with parallel calls; preserve permission/hooks/readonly/hashline boundaries.
    - Test settings/UI/prompt visibility and runtime versus saved state across new/resume/rebind/headless.
    - Format only changed Go files; run lint for changes in affected packages, tests of affected packages, and targeted race tests. Never run repository-wide gates. Review and commit only owned files in the task worktree.
created_at: "2026-09-05T14:13:16.34315Z"
updated_at: "2026-09-05T15:43:12.140449Z"
---

## Body

Implement the approved planning redesign. General Enable planning defaults true and is process-start only; show exact successful-save notice: «Чтобы применить настройку, закройте CozyPhi и начните новую сессию». Remove sidebar Plan/Require plan. Default build is voluntary planning; useplan is the sole strict execution mode. Persist last build/useplan, with plan a temporary readonly authoring posture. Use one immutable policy per tool round. Preserve safety gates and durable plans. Migration: explicit plan.enabled wins, otherwise preserve legacy PlanDisabled opt-out; never migrate old on to useplan. Cover engine replacements, rebind/model switches, headless, children, prompts, help and all UI entrances. Code and checks only in .worktrees/planning-availability-useplan; main carries ledger only.

**Started (2026-09-05).** Approved implementation plan. Starting from main 49bd007; preserve foreign go.sum and light-theme task changes. Code work isolated in task worktree.

**Note (2026-09-05).** Runtime/UI/docs implemented in task worktree. User explicitly narrowed all gates to changed files and affected packages (no full-repository make gates). Two-axis review found explicit lifecycle skill loss and live catalog reads across a round; both reproduced and fixed with regression tests, including three-call batches switching posture both ways. Exact restart notice corrected. Final scoped checks/review in progress; no code commit yet.

**Done (2026-09-05).** Implemented and committed as ae12f6f on feature/planning-availability-useplan in .worktrees/planning-availability-useplan; worktree clean. Startup-only General Enable planning, default soft build, strict useplan, temporary plan restoring execution preference, immutable round policy/catalog, sidebar toggle removal, runtime-off UI/tool hiding, persistence/migration, docs and exact restart notice. Standards/Spec review findings fixed and follow-up review found no blockers. Scoped gates green: formatter only changed Go files; lint --new-from-rev=HEAD over 11 affected packages (0 issues); tests all 11 packages; race agent/controller/harnesssettings/plangate. No repository-wide gates. Main code and unrelated changes untouched; branch retained without merge per task isolation constraint.

## Acceptance Criteria

- Planning defaults enabled with build/soft; useplan alone enforces plans; no sidebar planning checkbox.
- Global enablement is saved and startup-only; execution build/useplan is persisted and applied atomically next round.
- Soft removes all planning restrictions without bypassing permissions, hooks, read-only roles or input validation; off removes planning surfaces but retains plans.
- Explicit settings win over legacy/defaults; legacy enabled never implies useplan.
- Regression tests, documentation and changelog updated; verified changes committed in isolated worktree.

## Verification Plan

1. Test persistence/defaults/migration/conflicts/failed saves.
2. Test off/soft/useplan, lifecycle and per-round atomicity with parallel calls; preserve permission/hooks/readonly/hashline boundaries.
3. Test settings/UI/prompt visibility and runtime versus saved state across new/resume/rebind/headless.
4. Format only changed Go files; run lint for changes in affected packages, tests of affected packages, and targeted race tests. Never run repository-wide gates. Review and commit only owned files in the task worktree.

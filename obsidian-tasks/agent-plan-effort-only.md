---
id: agent-plan-effort-only
title: Restrict agent and plan model controls to effort
status: done
priority: high
model_level: high
task_type: feature
branch: feature/agent-plan-effort-only
worktree_path: .worktrees/agent-plan-effort-only
acceptance_criteria:
    - agent_spawn and plan expose effort only, never executable model selection
    - User-configured models remain authoritative; legacy model inputs cannot override them
    - Scoped checks pass; code and task ledger are committed
verification_plan:
    - Trace tool schemas and runtime resolution
    - Add boundary and runtime regression tests
    - Run formatting, tests and lint scoped to changed packages
created_at: "2026-09-05T12:24:24.658675Z"
updated_at: "2026-09-05T13:11:22.477411Z"
---

## Body

Remove model selection from model-facing agent_spawn and every plan authoring operation. Allow only effort overrides, while retaining user/harness model configuration and compatibility with stored plans.

**Note (2026-09-05).** Implemented effort-only model-facing plan/spawn contracts with runtime guards, saved-plan compatibility, lifecycle restoration and human UI precedence. Standards/spec review findings fixed. Scoped formatting, tests of all 10 changed packages and build passed; final scoped lint running before commit/integration.

**Done (2026-09-05).** Implemented in 0e83b86 and merged into main: effort-only schemas and runtime rejection of model authoring across plan/spawn paths, preserved user/legacy pins, restored effort across lifecycle, integrated human picker/display; docs and changelog updated. Scoped formatting/build and all 10 changed-package tests passed. Single scoped lint reported 10 findings; all fixed and scoped tests passed afterward; lint not rerun per one-run policy. No push.

## Acceptance Criteria

- agent_spawn and plan expose effort only, never executable model selection
- User-configured models remain authoritative; legacy model inputs cannot override them
- Scoped checks pass; code and task ledger are committed

## Verification Plan

1. Trace tool schemas and runtime resolution
2. Add boundary and runtime regression tests
3. Run formatting, tests and lint scoped to changed packages

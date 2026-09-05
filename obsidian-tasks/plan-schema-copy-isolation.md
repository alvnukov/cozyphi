---
id: plan-schema-copy-isolation
title: Keep injected plan schemas isolated from input tool definitions
status: done
priority: high
model_level: high
task_type: bug
parent_id: multisession-runtime-split
tags:
    - plan
    - race
    - isolation
branch: bug/plan-schema-copy-isolation
worktree_path: .worktrees/plan-schema-copy-isolation
acceptance_criteria:
    - InjectPlanStep does not mutate input tool definitions, Params pointers, Properties maps or Required slices.
    - Different sessions/policies can inject plan_step concurrently without schema/data races.
    - Nil Params are handled without panic and existing required/voluntary policy semantics remain correct.
verification_plan:
    - Public InjectPlanStep tests prove source schemas untouched, required/voluntary outputs independent, nil Params safe.
    - Concurrent injection and marshal test passes under race.
    - Run targeted agent TestSettersDuringLoopAreRaceFree under race.
created_at: "2026-09-05T15:52:04.037318Z"
updated_at: "2026-09-05T16:47:35.597474Z"
---

## Body

During multisession runtime implementation the agent worker reported TestSettersDuringLoopAreRaceFree failing because plangate.Policy.InjectPlanStep writes shared schema properties while inference marshals them. Source confirms a shallow Tool copy followed by writes through Definition.Params at internal/plangate/plangate.go:317-348; nil Params initialization also does not update the already-copied output. This is a prerequisite safety fix for per-session tool snapshots, implemented within the runtime worktree; do not defer the shared-state race to UI.

**Started (2026-09-05).** Prerequisite subtask of active multisession-runtime-split. Implementation is scoped to internal/plangate in the parent's isolated .worktrees/multisession-runtime-split worktree and will land with that verified runtime slice; no separate worktree or main code edits.

**Done (2026-09-05).** Landed with runtime code commit 0d903f9 and local main integration. InjectPlanStep clones the modified parameter/map/slice layers and handles nil schemas without mutating shared tool definitions. Public isolation/concurrency tests, plangate race tests and the previously failing agent setter race regression passed; post-format/lint-fix targeted tests passed. Nested schema values remain read-only. Implemented in the parent runtime worktree; no separate worktree or push.

## Acceptance Criteria

- InjectPlanStep does not mutate input tool definitions, Params pointers, Properties maps or Required slices.
- Different sessions/policies can inject plan_step concurrently without schema/data races.
- Nil Params are handled without panic and existing required/voluntary policy semantics remain correct.

## Verification Plan

1. Public InjectPlanStep tests prove source schemas untouched, required/voluntary outputs independent, nil Params safe.
2. Concurrent injection and marshal test passes under race.
3. Run targeted agent TestSettersDuringLoopAreRaceFree under race.

---
id: hook-denial-test-load-timeout
title: Stabilize hook denial fixture under concurrent race load
status: todo
priority: low
model_level: medium
task_type: bug
tags:
    - tests
acceptance_criteria:
    - Hook denial test remains deterministic under concurrent race-suite load without masking real hook failures.
verification_plan:
    - Reproduce TestLoadedHooksDenyBash under concurrent suite load.
    - Run focused hook tests with race detector and controlled scheduling.
created_at: "2026-09-05T20:44:15.791249Z"
updated_at: "2026-09-05T20:44:15.791249Z"
---

## Body

During interactive-child final scoped race checks, TestLoadedHooksDenyBash failed because its shell fixture exceeded its 5s timeout, returning fail_closed timeout rather than expected deny reason. No data race reported. Unchanged fixture; isolated -race -count=3 passed in 2.129s. Evidence: /tmp/cozyphi-child-final-races.log. Diagnose fixture scheduling/load sensitivity separately; do not simply weaken production hook timeout behavior.

## Acceptance Criteria

- Hook denial test remains deterministic under concurrent race-suite load without masking real hook failures.

## Verification Plan

1. Reproduce TestLoadedHooksDenyBash under concurrent suite load.
2. Run focused hook tests with race detector and controlled scheduling.

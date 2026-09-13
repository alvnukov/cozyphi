---
id: autonomous-wake-web-taint
title: Preserve web taint across autonomous wake turns
status: done
priority: high
model_level: high
task_type: bug
parent_id: event-driven-wait-monitor
tags:
    - issue
    - security
    - web-taint
    - notifications
branch: codex/event-driven-wait-monitor
worktree_path: ../event-driven-wait-monitor
acceptance_criteria:
    - Only actual user input resets web taint.
    - Autonomous and unknown origins retain the mark and cannot grant permission.
    - Background Bash and shell_task stop retain mutating permission checks.
verification_plan:
    - Use the completed focused origin/gate race checks and final project gates recorded in the body.
created_at: "2026-09-13T09:14:27.785119Z"
updated_at: "2026-09-13T09:15:17.710282Z"
---

## Body

Discovered and fixed while implementing [event-driven-wait-monitor](event-driven-wait-monitor.md). Engine.Loop previously reset web taint on every turn, allowing watch, child or shell completion to clear the mark despite no human input. Such completion events cannot grant new user authorization.

Resolution: explicit TurnOrigin records actual dispatch source; only TurnUserInput resets taint. Autonomous and unknown origins preserve it. Main interactive, queued user, and headless user dispatch are explicit; background and plan-resume dispatch remain autonomous. Background Bash and shell_task stop still pass the permission and mutating web-taint gates; list/get remain read-only.

Existing verification passed: TestAutonomousAndUnknownTurnsPreserveWebTaint, TestActualUserOriginResetsTaintRegardlessOfPromptText, TestShellTaskActionRemainsVisibleToWebTaintGate, and TestBackgroundBashCannotReuseForegroundAllowlist. Focused race command 293db43bab8f4c34e10d4710a4a160cc passed. Final make fmt-check db1284b40a3ef1c6e3758241bf1f4232, make lint 2e36bf62191c00141d01328649fd9200 (0 issues), and make test d629a3f3af244a70424012d9796022f3 passed. Existing manual-user reset assertions were preserved; fixtures now state their actual origin. Independent security review found no outstanding substantive issue. This registry record documents the completed fix; no additional code changes or repeated checks were required.

## Acceptance Criteria

- Only actual user input resets web taint.
- Autonomous and unknown origins retain the mark and cannot grant permission.
- Background Bash and shell_task stop retain mutating permission checks.

## Verification Plan

1. Use the completed focused origin/gate race checks and final project gates recorded in the body.

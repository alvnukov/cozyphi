---
id: event-driven-wait-monitor
title: Add background Bash and event-driven wait and monitor notifications
status: done
priority: high
model_level: high
task_type: feature
tags:
    - issue
    - feedback
    - bash
    - watch
    - monitor
    - notifications
    - claude-code
branch: codex/event-driven-wait-monitor
worktree_path: ../event-driven-wait-monitor
acceptance_criteria:
    - Verify the current official Claude Code documentation and record the actual background Bash, waiting, monitoring, and notification behavior with source links before fixing the CozyPhi contract.
    - 'Provide explicit background Bash execution with a stable handle and a visible lifecycle: running/completed/failed/stopped status, bounded output access, and stop/cancel behavior.'
    - Wake the model on meaningful monitor events or background command completion without requiring model-issued polling commands; preserve idle responsiveness and avoid lost or duplicate completion notifications.
    - Reuse the existing watch and session event/reminder mechanisms where applicable; keep notifications distinguishable from user messages.
    - Preserve permission checks, cancellation, process cleanup, output/event/resource bounds, and automatic-turn limits; maintain existing foreground Bash and watch behavior.
    - Document the user-visible tools and behavior, add an Unreleased changelog entry, and pass focused lifecycle/integration checks plus required repository gates.
verification_plan:
    - Inspect existing Bash, watch, engine wake/event delivery and public tool registration before editing; use happ calls on functions being changed.
    - Add or update focused tests for background success/failure, notification wake and ordering, output bounds, cancellation, permission denial, and cleanup as warranted by the implementation.
    - Run the narrow affected Go package tests first; use race/integration checks where concurrent lifecycle changes introduce concrete risk.
    - Run make fmt and the required make fmt-check lint test gates, scoped where justified, and happ diagnostics for touched Go files; close only with validated implementation and the owned-files task workflow.
created_at: "2026-09-13T08:01:18.746696Z"
updated_at: "2026-09-13T09:15:17.707452Z"
---

## Body

Implement in codex/event-driven-wait-monitor, worktree /Users/zol/src/cozyphi/.worktree/event-driven-wait-monitor. Approved design: one internal/shelltask process owner on Runtime/app, shared by interactive Controllers. Bash run_in_background returns task ID/output_file; Ctrl+B promotes the same process without restart. Foreground default300/max3600 seconds unchanged; explicit background without timeout lasts until app exit, explicit timeout1..3600 is enforced, promotion retains its deadline. shell_task list/get/stop is scoped to originating ParentSessionID; get reads at most8000 bytes of owned in-memory output, Read(output_file) retains normal filesystem gate. UI snapshots cover all app tasks with origin; background survives clear/resume/tab closure, notification only wakes original session. Runtime.Close cancels and joins processes and bounded artifact cleanup, then removes only its private shells-* output store; shared jobs/shell and other Runtime stores remain untouched. Output paths are valid until acknowledged-history eviction or this Runtime exits.

Terminal result.json is persisted before coalesced change hints; current controller wake/Inbox and session.AppendDelivery persist and deduplicate receipts before source ack. No lossy watch event carries terminal truth. Existing watch handles stream monitors, changed-output polling and timers; existing five-turn wake streak applies. Notification explicitly is untrusted command output, not human input/approval. Resource bounds:8 live processes and128 retained background records, first8MiB private output file/32KiB tail, dirs0700/files0600. Foreground records and full result memory retire after their native result is returned. History pressure evicts only oldest acknowledged terminal background entries; pending receipts are protected. A bounded128-item artifact cleanup worker is joined by Close, and pressure refuses new background work without blocking ordinary foreground Bash. Pending delivery belongs to the current Runtime only, with no automatic recovery after restart. Delivered receipts replay from session history; a historical launch without a terminal receipt is honestly status unavailable. Processes never restart from receipts. POSIX managed process-group cleanup is opt-in in proc.Spec; detached setsid/daemon containment is unsupported. Windows cancellation remains best effort.

Verified official sources on 2026-09-13: https://code.claude.com/docs/en/tools-reference#monitor-tool and https://code.claude.com/docs/en/interactive-mode#background-bash-commands . Claude exposes background Bash, Ctrl+B, task ID/output_file/terminal notifications; TaskOutput is deprecated in favor of Read. Separate doc records reference behavior and scope.

Adjacent security defect found during implementation: Engine.Loop unconditionally reset web taint for watch/child/shell autonomous wakes despite no human input. Fix by explicit TurnOrigin enum with unknown fail-closed; only actual user dispatch resets taint. Background Bash and shell_task stop remain mutating under web-taint gate. Add regression tests; do not weaken existing checks.

Implementation, integration and final review complete. Release decision accepted; local owned-files finalization uses the already successful checks below, with no source changes or repeated test execution. Independent read-only core/retention/replay review verified the correction of full-output retention and bounded tail copying, with no outstanding substantive finding. Focused lifecycle, real same-PID promotion, process-group cleanup, explicit origin/gate, idle/busy/order/budget delivery, and clear/resume tests pass. Race checks passed for shelltask, Bash adapter, permission, agent, and controller; targeted private-store cleanup and immediate-completion ordering checks also passed. TestOnlyInteractiveParentEngineReceivesShellCapability and TestBackgroundCapabilityIsNotAdvertisedOrAcceptedWithoutOwner verify parent/child registration and standalone schema rejection. Source inspection confirms background prompt guidance is conditioned on the same available-manager capability; no dedicated prompt assertion was added. Existing exact catalog assertions were updated for shell_task; the 4100-character prompt budget and security assertions remain unchanged.

Final project evidence (2026-09-13): make fmt PASS (command 60f6ef4d865ba523d6f24c0de78862bf); make fmt-check PASS (db1284b40a3ef1c6e3758241bf1f4232); make lint PASS, 0 issues (2e36bf62191c00141d01328649fd9200); make test PASS (d629a3f3af244a70424012d9796022f3); git diff --check PASS (49e9a6c911f10a77368ddb622520d12d). Relevant race run 293db43bab8f4c34e10d4710a4a160cc and final output/cleanup/order race run 3172aa8f5e423cdea4e13b1a81d82a65 passed. All touched Go files received happ diagnostics; some cached cross-file symbol diagnostics remained stale despite successful compilation, race tests and repository gates. Removed only the verified tooling-added objx go.mod checksum from this worktree; no dependency change or root-checkout modification. Finalized together with related fixed issue [autonomous-wake-web-taint](autonomous-wake-web-taint.md) in one local owned-files commit. No push or merge.

source_repo_path: /Users/zol/src/cozyphi

## Acceptance Criteria

- Verify the current official Claude Code documentation and record the actual background Bash, waiting, monitoring, and notification behavior with source links before fixing the CozyPhi contract.
- Provide explicit background Bash execution with a stable handle and a visible lifecycle: running/completed/failed/stopped status, bounded output access, and stop/cancel behavior.
- Wake the model on meaningful monitor events or background command completion without requiring model-issued polling commands; preserve idle responsiveness and avoid lost or duplicate completion notifications.
- Reuse the existing watch and session event/reminder mechanisms where applicable; keep notifications distinguishable from user messages.
- Preserve permission checks, cancellation, process cleanup, output/event/resource bounds, and automatic-turn limits; maintain existing foreground Bash and watch behavior.
- Document the user-visible tools and behavior, add an Unreleased changelog entry, and pass focused lifecycle/integration checks plus required repository gates.

## Verification Plan

1. Inspect existing Bash, watch, engine wake/event delivery and public tool registration before editing; use happ calls on functions being changed.
2. Add or update focused tests for background success/failure, notification wake and ordering, output bounds, cancellation, permission denial, and cleanup as warranted by the implementation.
3. Run the narrow affected Go package tests first; use race/integration checks where concurrent lifecycle changes introduce concrete risk.
4. Run make fmt and the required make fmt-check lint test gates, scoped where justified, and happ diagnostics for touched Go files; close only with validated implementation and the owned-files task workflow.

---
id: multisession-interactive-children
title: Retain interactive child sessions with turn-scoped interruption
status: done
priority: high
model_level: high
task_type: feature
parent_id: multisession-mode
tags:
    - multisession
    - agents
branch: feature/multisession-interactive-children
worktree_path: .worktrees/multisession-interactive-children
acceptance_criteria:
    - Native interactive children retain a Controller/Session and reuse the multisession registry; headless runner remains compatible.
    - SessionID/JobID/TurnID and parent linkage are structured; interrupt cancels one turn, while leave-after-interruption stops the assignment once without discarding history.
    - Queued input, permissions and continuation stay origin-scoped; role ceilings, cancellation and concurrency limits hold in race tests.
verification_plan:
    - Public-interface lifecycle tests with controlled orderings for interrupt/submit/leave/finish and queued text-only turns.
    - Cross-session and stale interaction rejection; read-only roles, cancellation, timeout and resource budgets.
    - Focused agent/job/controller tests and race checks; existing headless spawn/wait/cancel compatibility.
created_at: "2026-09-05T15:19:15.031841Z"
updated_at: "2026-09-05T21:18:32.184933Z"
---

## Body

Implement the approved lifecycle contract in obsidian-tasks/interactive-child-sessions-design.md. No separate session registry or prompt scheduler. Native interactive and headless runners form the real adapter seam. Serialize submit, interrupt, leave and finish, prevent late revival, and retain terminal results independently from conversation history. **Blocked by:** multisession-runtime-split, multisession-registry. **Effort:** high (maximum supported). Code only in the task worktree.

**Started (2026-09-05).** Starting the next approved narrow lifecycle/outcomes vertical after retained Views landed at 8a76e64. Medium implementation/test effort; no new workers or architecture detours. Code will use the task worktree, preserving explicit waits/headless behavior.

**Note (2026-09-05).** Work in .worktrees/multisession-interactive-children is uncommitted. Added Controller assignment lifecycle (turn interrupt retains assignment; continuation wins over leave; leave-after-interrupt produces typed stop), runtime interactive adapter retaining children and awaiting complete View attachment, shared role-bound child preparation preserving headless runner, and durable child/session/spawn-call correlation plus stop reason in job metadata. Terminal cmd reconciles children on the existing draw scheduler without auto-selection. Two scoped race runs passed, latest covers job/controller/agent/sessions/editor/cmd (/tmp/cozyphi-child-retained-integration.log); shared normal/child session-admission counter edited afterwards. No lint run yet. NOTES updated with observed edit-rebase and test-server cancellation issues. Still incomplete: durable parent inbox/outcome wake, linked follow-up jobs, remaining lifecycle/generation edge tests and real child terminal smoke. No new workers, no commit/merge/push.

**Note (2026-09-05).** Continued the same uncommitted lifecycle/outcome vertical. Added durable correlated terminal envelopes with bounded UTF-8 summaries, owner/conversation-scoped queries and acknowledgements; parent context persists delivery identity before ack. Interactive idle wake and safe inference-boundary delivery reuse the watch timer/streak, without putting outcomes into the lossy watch queue. Full scoped race passed for job/session/agent/controller/sessions/editor/cmd (w12, /tmp/cozyphi-child-outcome-integration.log). Later explicit wait/push dedup was RED (duplicate summary), then passed under race with native host-only delivery metadata and pre-wake reconciliation of stale hints. Interrupt-retention and /clear origin-fencing regressions also passed. NOTES updated. Still incomplete: terminal follow-up jobs, intervention metadata, persistence-failure reconciliation/error visibility, remaining lifecycle/burst/late-boundary coverage and role/global-policy verification, independent review, single lint and real child terminal smoke. No lint run, commit, merge or push yet.

**Note (2026-09-05).** Added RED→GREEN storage-failure regression: failed terminal metadata retains the existing job and admission slot, emits a parent hint, and exposes actionable undelivered errors through Wait/PendingOutcomes. Query-driven reconciliation preserves outcome identity after storage recovery; bounded in-process fallback, not crash durability while storage is unwritable. W13 found an Esc-recall fixture timing gap (!IsStreaming before actual loop exit); added !RunActive to its wait, 30 isolated races passed. W14 passed all seven scoped race packages (/tmp/cozyphi-child-receipt-recovery-races-fixed.log). Subsequently implemented terminal child follow-up through the same Manager/Controller: retained session/history/View, fresh PreviousJobID-linked assignment, unchanged old Wait result, linkage delivered to parent, visible admission errors. New follow-up test was RED then GREEN; combined targeted lifecycle/role/outcome/follow-up races passed. W14 predates follow-up edits. Remaining: reservation/admission/interrupt/close edges, queued-message visibility after stop, intervention marker, worker Ask/global-policy checks, shutdown persistence errors and remaining delivery/recovery tests, independent review, one lint, terminal smoke and integration. NOTES updated; all code remains uncommitted; go.sum excluded; no live watch, lint run, merge or push.

**Note (2026-09-05).** Continued narrow lifecycle hardening. Manager.Close/CloseOwner/CloseParent now return scoped undelivered persistence errors after runner exit and retry final persistence on repeated close; three-scope regression was RED (nil errors), then GREEN under race. Runtime callers still only debug-log those errors, so user-facing shutdown reporting remains incomplete. Fixed child initGate resetting configured policy to defaults: a real-read control/deny test proved configured path-deny leakage before the fix. Child assembly now loads workspace policy and applies role/plan ceilings, write confinement and no memory exemption without dropping configured rules; mode/allow-all regression passes. W15 all seven scoped race packages passed (/tmp/cozyphi-child-policy-shutdown-races.log). Additional worker Ask regression passed under race: child-origin approval only, parent bypass not inherited, no execution before approval, stale approval after interrupt cannot execute. NOTES updated through239. Remaining reservation/input/stop ordering, intervention metadata, remaining delivery/recovery, shutdown visibility, independent review, single lint and terminal smoke. No commit/merge/push, no live watches; go.sum remains excluded.

**Note (2026-09-05).** Core remains uncommitted in owned worktree. Completed independent Standards/Spec reviews and fixed reusable attachment readiness, actual-cleanup child retirement, final-boundary wait/push dedup, complete explicit-wait envelopes, reservation interruption, intervention metadata, follow-up bind failure and shutdown error propagation. Real tmux spawn/push/retained follow-up/draft smoke passed before latest review fixes. Foreign/unpublished metadata inbox-isolation regression now GREEN using immutable scope attribution. W19 formatting/full suite and W20 scoped race checks running; single lint still unused. Preserve unrelated go.sum, main ledger and parallel tasks; no push. Ignored NOTES updated through261.

**Note (2026-09-06).** Core committed a30e1dc; main207ec6f integration in task worktree in progress. W25 merged controller/sessions/session/cmd race gates passed. Full integration initially found only delivery test reopening a live owned history; fixture now releases owners. Focused merge review found queued terminal-child input lost during clear/resume; RED->GREEN regression covers clear/rollback and linked assignment execution, targeted races count3 pass. Reserved/interrupted assignments now reject conversation replacement (RED->GREEN). Final full integrated tests/build running w27. One lint allowance used; no rerun. Baseline lint and loaded hook-timeout findings tracked separately. Unrelated go.sum and parallel task notes preserved; no push.

**Done (2026-09-06).** Landed on main as1c1b11a (core a30e1dc; integration10cc7b6/c395b18). Retained interactive child lifecycle, interrupt/stop/follow-ups, role/workspace ceilings, durable parent outcomes and headless wait compatibility implemented. Fresh-start smoke caught/fixed opt-in ordering before first engine; verified automatic parent wake, selectable retained child, linked follow-up and receipts; process exited. Full suite w27 passed; build corrected to ./cmd; merged lifecycle races w25 and focused switch races count3 passed; concurrent settings delta w28 affected tests passed. One lint used: owned findings fixed, three baseline findings tracked multisession-baseline-lint-cleanup; no rerun. Hook-load timeout tracked separately. No push. Atomic model selection and selector UI remain separate approved steps, not claimed complete.

## Acceptance Criteria

- Native interactive children retain a Controller/Session and reuse the multisession registry; headless runner remains compatible.
- SessionID/JobID/TurnID and parent linkage are structured; interrupt cancels one turn, while leave-after-interruption stops the assignment once without discarding history.
- Queued input, permissions and continuation stay origin-scoped; role ceilings, cancellation and concurrency limits hold in race tests.

## Verification Plan

1. Public-interface lifecycle tests with controlled orderings for interrupt/submit/leave/finish and queued text-only turns.
2. Cross-session and stale interaction rejection; read-only roles, cancellation, timeout and resource budgets.
3. Focused agent/job/controller tests and race checks; existing headless spawn/wait/cancel compatibility.

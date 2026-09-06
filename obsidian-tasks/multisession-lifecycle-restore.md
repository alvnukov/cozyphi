---
id: multisession-lifecycle-restore
title: Prove tab and child-work shutdown; defer automatic restore
status: todo
priority: medium
model_level: high
task_type: feature
parent_id: multisession-mode
tags:
    - cozyphi
    - tui
    - multisession
branch: feature/multisession-lifecycle-restore
worktree_path: .worktrees/multisession-lifecycle-restore
acceptance_criteria:
    - Closing a working tab requires confirmation bound to its live identity; cancel preserves work. Confirm seals new work, stops owned work and closes without deleting history or affecting unrelated tabs.
    - Owned child assignments and independent work started from retained children are accounted for. A completed retained child running a controlled local shell is included in the parent-close cancellation/join test; no success is claimed while that owned work remains live.
    - The UI remains responsive during cleanup; a closing slot is not released before owned cleanup completes. Failures/timeouts are surfaced honestly and do not imply cancellation succeeded.
    - Graceful app exit warns about active work and joins all sessions under a shared bounded wait; it does not serialize a fresh full timeout per tab. Preserve last-tab close refusal and history ownership rules.
    - Session/child history and final result publication remain inspectable after confirmed close; no stale callback or queued follow-up resurrects closed ownership. Automatic restore and recent-session UI are not required.
verification_plan:
    - Use controlled parent/child runners and local-shell lifetime adapters; first test completed-child shell work during parent close, then delayed child result publication and stale follow-up callbacks.
    - Exercise cancel/confirm for background and foreground targets, a delayed/uncooperative worker, close admission races, last-tab refusal and independent sibling survival. Assert lifecycle barriers, not just eventual registry length.
    - Verify history and final result visibility after cleanup; inspect shutdown failure/timeout reporting and shared app-exit budget with deterministic synchronization.
    - Run tests/race/build/format only for changed lifecycle/job/UI packages; one scoped lint at most. No live providers, external process-killing smoke or automatic restoration of real histories.
created_at: "2026-09-04T07:31:55.433694Z"
updated_at: "2026-09-06T14:01:56.321277Z"
---

## Body

**What to deliver.** Prove and complete V1 stop-and-close for a session and its owned child work. The stable task ID is retained to avoid duplicate lifecycle tickets; its old automatic-restore half is deferred below and is not part of this task's completion criteria.

**SOURCE baseline (1a4cf31; relevant source unchanged at 05ce564).** Editor already captures close target identity, confirms working close, refuses the last tab, seals idle admission and waits asynchronously before removing a retained slot. Controller cleanup cancels streams/lifetimes and calls CloseOwner for its assignments; history files are not deleted. Shell exit already joins views concurrently. Do not replace this with synchronous cancellation, a new workspace refcount design or /clear on the last tab.

**Unverified guarantee to falsify first.** Parent assignment cancellation is not proof that every retained child Controller lifetime stopped. Start a cancellable local ! command from a completed retained child, then request parent close: confirmation must account for that owned work, cancellation must reach it and completion must await its exit. This is an inferred gap from ownership paths, not a reproduced bug. Preserve the approved interactive-child interruption/follow-up semantics while fixing only demonstrated gaps. Closing a child also needs a controlled delayed final-job-publication test before treating tab removal as a final-result barrier.

**Remaining path.** Cover parent stream, queued prompts, tools, local shell, watches and child work through close admission → confirmation → cancellation → join → history/result visibility. Use immutable live ownership rather than mutable tab position or history alone. Retained child transcripts may remain inspectable, but closed parent ownership must not permit continued hidden work or follow-up resurrection. Graceful quit must disclose active work, then use the shared wait; timeout/failure remains explicit and cannot be reported as clean termination. No daemon/reattach or guarantee that arbitrary external processes stop gracefully after terminal crash.

**Deferred restore proposal.** The former UIState open-session set, order/active-tab persistence, sessions.restore configuration, recent closed-session list and -c/--resume integration are future scope only. No default-on restore is approved by this V1. If selected later, restore views/history without restarting autonomous assignments, preserve model/effort semantics and skip missing paths safely; first agree UX/defaults and coordinate [multisession-projects](multisession-projects.md) plus [job-recovery-process-ownership](job-recovery-process-ownership.md). Do not add that work merely because this ID contains restore.

**Blocked by:** [multisession-registry](multisession-registry.md), [multisession-interactive-children](multisession-interactive-children.md), [multisession-parent-inbox](multisession-parent-inbox.md) (all recorded done). Can proceed alongside project opening; no sidebar/hotkeys/restore prerequisite. Final two-project integration belongs to [multisession-hardening](multisession-hardening.md).

## Acceptance Criteria

- Closing a working tab requires confirmation bound to its live identity; cancel preserves work. Confirm seals new work, stops owned work and closes without deleting history or affecting unrelated tabs.
- Owned child assignments and independent work started from retained children are accounted for. A completed retained child running a controlled local shell is included in the parent-close cancellation/join test; no success is claimed while that owned work remains live.
- The UI remains responsive during cleanup; a closing slot is not released before owned cleanup completes. Failures/timeouts are surfaced honestly and do not imply cancellation succeeded.
- Graceful app exit warns about active work and joins all sessions under a shared bounded wait; it does not serialize a fresh full timeout per tab. Preserve last-tab close refusal and history ownership rules.
- Session/child history and final result publication remain inspectable after confirmed close; no stale callback or queued follow-up resurrects closed ownership. Automatic restore and recent-session UI are not required.

## Verification Plan

1. Use controlled parent/child runners and local-shell lifetime adapters; first test completed-child shell work during parent close, then delayed child result publication and stale follow-up callbacks.
2. Exercise cancel/confirm for background and foreground targets, a delayed/uncooperative worker, close admission races, last-tab refusal and independent sibling survival. Assert lifecycle barriers, not just eventual registry length.
3. Verify history and final result visibility after cleanup; inspect shutdown failure/timeout reporting and shared app-exit budget with deterministic synchronization.
4. Run tests/race/build/format only for changed lifecycle/job/UI packages; one scoped lint at most. No live providers, external process-killing smoke or automatic restoration of real histories.

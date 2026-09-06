---
id: multisession-hardening
title: Verify cross-project tabs V1 with offline lifecycle scenarios
status: blocked
priority: high
model_level: very_high
task_type: test
parent_id: multisession-mode
tags:
    - cozyphi
    - tui
    - test
    - multisession
branch: test/multisession-hardening
worktree_path: .worktrees/multisession-hardening
acceptance_criteria:
    - Offline scenarios A–H exercise the actual public tab/session routes with controlled runners and two temporary projects; each assertion identifies its session/request owner.
    - Cancellation/join and capacity checks use owned lifecycle barriers; an uncooperative worker or cleanup error cannot be mistaken for successful shutdown. Existing histories remain usable.
    - 'V1 invariants are documented: tabs, canonical workspace vs shared repository resources, origin-bound asks, stop-and-close, no daemon/automatic restore and honest shutdown limits.'
    - The docs and CHANGELOG describe implemented behavior only; source-only conclusions, skipped terminal/device tests and the separate cross-process recovery issue are explicit.
    - All required V1 delivery changes are merged and their scoped verification evidence recorded; optional sidebar/recent/restore work does not block this task. Local checks do not sweep unchanged packages.
verification_plan:
    - Map scenarios A–H to existing public-route tests and add only missing integration coverage with temporary projects and fake transports/runners.
    - Run targeted tests and race checks on packages changed by this task, reusing prior verified evidence rather than rerunning all repository gates; one scoped lint at most before commit.
    - Check documentation against observed behavior and test limits; label source, runtime, inferred and unknown claims distinctly.
    - Optional terminal-width/theme/notification/voice smoke must name the environment and use controlled fixtures; live provider or device access requires separate approval. Verify owned commits/merges and ledger closure without post-merge gate reruns.
created_at: "2026-09-04T07:31:55.434427Z"
updated_at: "2026-09-06T14:03:16.863537Z"
---

## Body

**What to deliver.** Final integration evidence for cross-project tabs V1, not a new round of implementation of every old UI proposal. Earlier slices must ship their own tests; this task combines them at the user-facing route and fixes only demonstrated integration defects.

**Required offline matrix.**
A. Open B from A with different project instructions and same-named files; verify captured provider prompt and tool results agree with each session, including plain /new, relative path and prompt rebuild/child context.
B. Run A and B simultaneously, switch repeatedly and preserve both transcripts, drafts, attachments and scroll; selection does not retarget execution or change model/assignment priority.
C. Background permission/question/continue while typing; select and reply, then deliver stale events. Only the originating live request resolves; no focus theft or approval by navigation.
D. Cancel and confirm close during controlled tool/shell/watch work; B stops while A survives. Include completed retained child with independent shell work and delayed final outcome publication from the lifecycle task.
E. Validate failed-open rollback, non-directory paths, symlink same-checkout warning, distinct worktrees, retained-owner resume and mismatched history/project handling.
F. Exercise the current retained capacity (12 at the source baseline), closing slots and late child/open callbacks; no capacity leak or resurrection.
G. Interleave switch with completion/error/ask; selection is not unread, and unread clears only when transcript content is actually viewed at bottom. Verify narrow/wide render and no-color identity.
H. Graceful app exit with active work warns and requests all owned stops under a shared bounded wait; delayed cleanup reports timeout/failure honestly and preserves ownership while the process lives. A later history open must not silently restart abandoned autonomous work. This is not automatic tab-set restoration.

**Evidence discipline.** Baseline source investigation is 1a4cf31, relevant code unchanged at 05ce564; existing shell/runtime/close tests were read, not executed during backlog refresh. Completed task notes carry their own historical execution evidence. Use controllable transports/clocks/barriers, not live providers, ambient credentials, arbitrary sleeps or a fragile global goroutine count. Track owned resource release and bound cooperative cleanup; an arbitrary uncooperative process cannot be promised to terminate by a wall-clock deadline. No new dependency merely to count goroutines.

**Documentation.** Update the existing TUI/project layout documentation and appropriate README/CHANGELOG only when behavior lands. Explain actual key table bindings, one-visible-tab/background behavior, shared-checkout warning, history retention and limits. Do not document the old left panel or sessions.restore as shipped. [job-recovery-process-ownership](job-recovery-process-ownership.md) remains a separate open cross-process issue: disclose and assess it before broad rollout rather than hiding recovery failures.

**Blocked by:** [multisession-projects](multisession-projects.md), [multisession-hotkeys](multisession-hotkeys.md), [multisession-switch-cues](multisession-switch-cues.md), [multisession-background-attention](multisession-background-attention.md), [multisession-lifecycle-restore](multisession-lifecycle-restore.md). Completed title/registry/runtime/child tasks remain prerequisites already satisfied; grouped panel, recent picker and automatic restore are not gates.

**Blocked (2026-09-06).** Final V1 sign-off waits for multisession-projects, multisession-hotkeys, multisession-switch-cues, multisession-background-attention and the V1 shutdown scope of multisession-lifecycle-restore. Earlier slices still carry their own tests. Deferred sidebar/recent/automatic restore are not blockers.

## Acceptance Criteria

- Offline scenarios A–H exercise the actual public tab/session routes with controlled runners and two temporary projects; each assertion identifies its session/request owner.
- Cancellation/join and capacity checks use owned lifecycle barriers; an uncooperative worker or cleanup error cannot be mistaken for successful shutdown. Existing histories remain usable.
- V1 invariants are documented: tabs, canonical workspace vs shared repository resources, origin-bound asks, stop-and-close, no daemon/automatic restore and honest shutdown limits.
- The docs and CHANGELOG describe implemented behavior only; source-only conclusions, skipped terminal/device tests and the separate cross-process recovery issue are explicit.
- All required V1 delivery changes are merged and their scoped verification evidence recorded; optional sidebar/recent/restore work does not block this task. Local checks do not sweep unchanged packages.

## Verification Plan

1. Map scenarios A–H to existing public-route tests and add only missing integration coverage with temporary projects and fake transports/runners.
2. Run targeted tests and race checks on packages changed by this task, reusing prior verified evidence rather than rerunning all repository gates; one scoped lint at most before commit.
3. Check documentation against observed behavior and test limits; label source, runtime, inferred and unknown claims distinctly.
4. Optional terminal-width/theme/notification/voice smoke must name the environment and use controlled fixtures; live provider or device access requires separate approval. Verify owned commits/merges and ledger closure without post-merge gate reruns.

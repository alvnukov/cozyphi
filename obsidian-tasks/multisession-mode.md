---
id: multisession-mode
title: 'Multi-session V1: cross-project tabs in one terminal'
status: todo
priority: high
model_level: very_high
task_type: epic
parent_id: cozyphi-convenience-program
tags:
    - cozyphi
    - tui
    - multisession
    - epic
acceptance_criteria:
    - Two live tabs in different projects retain independent conversation, model/effort selection, plan, draft, scroll and pending asks; switching does not restart ordinary background work or change its project.
    - The first slice opens project B through /new <path>; plain /new uses the active tab project. Prompt instructions, tools, session history and workspace-backed UI agree on the selected project; mismatched resume cannot execute with mixed identities.
    - Tabs identify their session and project. Reopening the same canonical checkout warns without creating a worktree. Background status/attention identifies its owner and does not steal input or authorize another session.
    - Closing a working tab requires genuine user confirmation, seals new work, stops and joins owned work including child work, and preserves history without affecting unrelated tabs. Graceful exit warns about active work; timeout is not reported as successful cleanup.
    - Existing title pinning, role ceilings, per-session model selection and the permission hook/gate order remain intact. Runtime ownership is reused rather than replaced.
    - V1 delivery tasks are verified and merged; deferred panel/recent/restore extensions are not V1 completion gates. Evidence and limitations are documented; local gates cover changed packages only.
verification_plan:
    - Use two temporary projects with different instructions and same-named files, controlled provider/runner adapters and no live credentials; verify the complete open/run/switch/ask/close path.
    - Verify the prerequisite graph and the merged evidence for V1 tasks; do not require deferred sidebar/restore tasks to be done.
    - Exercise invalid paths, symlink aliases, mismatched resume, delayed background events and owned child shutdown; report source-only or unknown guarantees explicitly.
    - Run format/build/test/race only on packages changed by each implementation; at most one scoped lint per task. Markdown-only maintenance uses link/metadata/diff checks, not Go gates. Live provider or device checks require separate approval.
created_at: "2026-09-04T07:31:55.423495Z"
updated_at: "2026-09-06T13:58:18.52001Z"
---

## Body

**Goal and authority (2026-09-06).** Deliver tabs for sessions across different projects in one terminal. One conversation is visible; others execute in the background only while CozyPhi is alive. This approved V1 contract supersedes the original left-sidebar mockup, mandatory Alt-key redesign and automatic restore requirements. It does not undo the completed interactive-child contract.

**V1 contract.**
- Reuse retained tabs and the existing selector; no split view, daemon, detach/reattach or automatic worktree creation.
- New work in another project must use that project's instructions and execution context, not merely display a different path. Shared checkout is allowed with a warning; symlink aliases count as the same canonical directory.
- Session-owned execution, pending permissions, transcript and editing state must not be retargeted by selecting another tab. Selection is not assignment priority and never grants approval. Shared process configuration remains shared; this is not a promise of per-tab copies of all settings.
- Working-tab close means confirm, stop and close, not hide and keep running. Preserve history. Prove child-work ownership as well as parent stream cancellation; retained child UI is not itself evidence that all child work has stopped. Preserve the current refusal to close the last tab; do not silently replace it with /clear. Graceful app exit must warn about active work and stop owned work; terminal crashes cannot guarantee graceful termination of arbitrary external processes.

**Existing implementation — SOURCE, not a new runtime test.** Investigation at 1a4cf31, relevant source unchanged at 05ce564, confirms Runtime.Workspace/NewSession, retained Registry/View/Bus, foreground draw/background drain, top selector, /switch N, next/previous/back bindings and confirmed asynchronous close. Nine completed child tasks remain done; their recorded test/merge evidence is historical, not rerun here. The latest focused attention delivery is recorded in [multisession-background-attention](multisession-background-attention.md). Do not reimplement these solely because larger UI tasks remain open.

**Ownership.** Process runtime coordinates shared resources. Workspace identity is canonical cwd, not simply git root: separate worktrees can have distinct configuration/MCP/LSP contexts while intentionally sharing memory and the task registry for one repository. Session owns its engine, history, model selection, plan, asks, watches and View state. Hooks may intentionally execute in their plugin directory while receiving the session context; do not flatten that contract into a blanket chdir. Preserve PreHooks → Gate/Ask → Run → PostHooks. Account-wide routing/budget policy, if later implemented, must remain shared; changing tab focus is not a routing priority change.

**First vertical slice.** [multisession-projects](multisession-projects.md) owns opening a second project from /new <path>, including the narrow explicit-session prompt correction before exposing that route. It reuses the existing assembly and identifies the project in the selector. [refactor-prompt-snapshot](refactor-prompt-snapshot.md) retains the broader pure-render/IO/panic refactor; its whole completion is not a prerequisite. Resume must resolve the correct workspace or reject a mismatch before execution. This first slice carries its own two-project regression checks; it need not wait for all final hardening.

**Remaining V1 deliveries.** [multisession-hotkeys](multisession-hotkeys.md) reconciles existing navigation/help without removing /switch; [multisession-switch-cues](multisession-switch-cues.md) verifies project/ask identity through switching; [multisession-background-attention](multisession-background-attention.md) verifies origin-bound attention; [multisession-lifecycle-restore](multisession-lifecycle-restore.md) now owns the V1 shutdown proof, with restore explicitly deferred. [multisession-hardening](multisession-hardening.md) is final offline V1 sign-off, not a prerequisite for every earlier delivery.

**Deferred, not silently deleted.** [multisession-sessions-panel](multisession-sessions-panel.md) preserves the optional grouped-sidebar proposal outside V1 and needs a fresh UX decision. Recent-project suggestions, all-project history picker/CLI listing and restoring an open-tab set are retained as future candidates in their existing notes, not counted as missing V1 features. Do not claim they exist. No default-on sessions.restore decision is carried forward.

**Other open issue.** [job-recovery-process-ownership](job-recovery-process-ownership.md) remains valid and unchanged. Cross-process coordination is outside this single-process V1, not permission to damage another process's live jobs. Before broad rollout evaluate that issue explicitly; do not close or suppress it because V1 tab tests pass.

**Delivery order.** Completed runtime/registry → projects; completed registry → hotkeys, attention and lifecycle work in parallel; projects → switch cues; projects + hotkeys + cues + attention + lifecycle → hardening. These are human-readable prerequisites, not an executable workflow DSL. Optional panel/restore work never blocks this chain. Epics stay open until their V1 contract is met; updating this ledger does not complete implementation.

**Historical note (2026-09-05; original child-slice approval, UI scope above takes precedence).** Implementation of the child-session slice is approved: full contract in interactive-child-sessions-design (commit 6aa5898), not a viewer-first feature. Order: runtime-split -> registry/full retained Views -> interactive-children -> parent-inbox -> selector/attention -> hardening; atomic per-session model/effort reuses refactor-engine-reconfigure. Existing sessions-panel/hotkeys/switch-cues/background-attention remain the UI tasks; dot means selection, not unread. Model changes apply atomically at the next inference request and preserve child role ceilings. New child-specific tasks: multisession-interactive-children and multisession-parent-inbox. General projects/restore/title-tool work is not prerequisite and must not be marked done by this slice. All code in task worktrees; effort high for architecture/lifecycle/model/inbox/hardening, medium for selector UI, low for bookkeeping. Scoped gates per changed packages; preserve unrelated edits.

## Acceptance Criteria

- Two live tabs in different projects retain independent conversation, model/effort selection, plan, draft, scroll and pending asks; switching does not restart ordinary background work or change its project.
- The first slice opens project B through /new <path>; plain /new uses the active tab project. Prompt instructions, tools, session history and workspace-backed UI agree on the selected project; mismatched resume cannot execute with mixed identities.
- Tabs identify their session and project. Reopening the same canonical checkout warns without creating a worktree. Background status/attention identifies its owner and does not steal input or authorize another session.
- Closing a working tab requires genuine user confirmation, seals new work, stops and joins owned work including child work, and preserves history without affecting unrelated tabs. Graceful exit warns about active work; timeout is not reported as successful cleanup.
- Existing title pinning, role ceilings, per-session model selection and the permission hook/gate order remain intact. Runtime ownership is reused rather than replaced.
- V1 delivery tasks are verified and merged; deferred panel/recent/restore extensions are not V1 completion gates. Evidence and limitations are documented; local gates cover changed packages only.

## Verification Plan

1. Use two temporary projects with different instructions and same-named files, controlled provider/runner adapters and no live credentials; verify the complete open/run/switch/ask/close path.
2. Verify the prerequisite graph and the merged evidence for V1 tasks; do not require deferred sidebar/restore tasks to be done.
3. Exercise invalid paths, symlink aliases, mismatched resume, delayed background events and owned child shutdown; report source-only or unknown guarantees explicitly.
4. Run format/build/test/race only on packages changed by each implementation; at most one scoped lint per task. Markdown-only maintenance uses link/metadata/diff checks, not Go gates. Live provider or device checks require separate approval.

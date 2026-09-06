---
id: multisession-projects
title: Open a second project tab with consistent session context
status: todo
priority: high
model_level: high
task_type: feature
parent_id: multisession-mode
tags:
    - cozyphi
    - tui
    - project
    - multisession
branch: feature/multisession-projects
worktree_path: .worktrees/multisession-projects
acceptance_criteria:
    - /new <path> opens and selects a new project tab; /new without a path uses the active tab's project. Relative paths resolve from that project, ~ expands predictably, paths containing spaces work, and invalid/non-directory paths fail without disturbing live tabs or retaining a partial session slot.
    - A and B use their own project instructions and declared workspace in system prompts, including fresh turns, child preparation, reconfiguration and compaction rebuilds; no global chdir or ambient process cwd selects the prompt project.
    - B's controller, engine history cwd, project configuration and View agree. Filesystem/bash tools, local shell, watches, hooks context, memory/tasks and MCP/LSP identity follow their documented session/workspace/repository scope, with no A/B leakage.
    - Opening an existing history selects its retained owner, constructs the correct workspace, or rejects a cwd mismatch before execution. A warning alone or merely removing cwdWarning is insufficient.
    - The selector exposes project identity; opening another session in the same canonical checkout, including a symlink alias, warns without creating a worktree. Different worktrees are not collapsed solely by shared git root.
    - Controlled two-project tests demonstrate simultaneous work, preserved drafts/transcripts, origin-bound asks and close of B without stopping A. History survives close; broad descendant-shutdown proof belongs to the linked lifecycle task.
verification_plan:
    - Exercise the actual /new route with temporary A/B projects, distinct AGENTS.md content and same-named files; capture provider input through a fake transport and execute only controlled local tools.
    - Keep process cwd at A while testing B prompt construction/rebuild, child preparation, file/bash/local-shell/watch context and workspace-backed UI identity. Use adapters rather than live MCP/LSP servers or credentials.
    - Test relative and spaced paths, invalid directories, symlink-alias warning, shared-repository worktrees, failed assembly rollback and mismatched resume rejection/rebinding.
    - Run concurrent A/B work with a background ask, switch during delivery, close B and verify A plus both histories. Reuse retained-session tests; link remaining descendant-lifetime cases to lifecycle/hardening.
    - Format/build/test/race only changed packages; one scoped lint at most before commit. Update user-visible docs/CHANGELOG with the implementation, not with ledger maintenance.
created_at: "2026-09-04T07:31:55.432981Z"
updated_at: "2026-09-06T13:59:02.416058Z"
---

## Body

**What to deliver.** A complete first slice: launch in A, open B with /new <path>, keep A running in the background, use B, return to A without losing state. Reuse the current Runtime.Workspace/NewSession and retained View assembly; do not add a second registry or make a left sidebar a prerequisite.

**SOURCE baseline (1a4cf31; relevant code unchanged at 05ce564).** Runtime already canonicalizes workspace directories and scopes project-backed resources. The ordinary session factory still captures the startup workspace/project, and /new rejects arguments. Engine.systemPrompt calls prompt.BuildWithFacts without a session cwd; that renderer loads project context through process os.Getwd. Resume retains the controller workspace and only warns on a different history cwd. These are source observations, not a live two-project reproduction.

**Implementation order inside this slice.** First make prompt project identity explicit using the session's effective context, before exposing cross-project creation. This narrow correction is owned here and linked to [refactor-prompt-snapshot](refactor-prompt-snapshot.md); do not wait for or duplicate that task's complete pure-render/IO/panic refactor. Then route /new through the existing assembly with one coherent selected Workspace, Controller, history and View. Relative paths and plain /new use the active project rather than the process startup directory. Finish with project identity, shared-checkout warning and the public-route regression scenario.

**Scope and authority.** Validate the selected directory and project trust using existing permission rules before executable project configuration runs; changing tabs cannot grant new authority. Preserve default gate/ask behavior, hook ordering and role ceilings. Never temporarily chdir the process to prepare B. Verify files/bash/local shell and watch cwd; hooks can execute in a plugin directory while receiving B's session context. MCP/LSP are workspace-scoped; memory/tasks may intentionally be repository-shared between worktrees. Do not turn these legitimate sharing rules into false per-tab-isolation claims. Existing project MCP trust hardening remains separately tracked; do not bypass it to make this slice pass.

**Resume guard.** All exposed resume paths must avoid mixed history/engine/controller/project identities. Reuse a retained owner where present; otherwise bind to the history's workspace or reject mismatches before work starts. Automatic restoration of a tab set and a new cross-project picker are not needed for this guard.

**Project display.** Provide the minimum clear project label in the existing selector as part of this slice. [multisession-switch-cues](multisession-switch-cues.md) consumes and hardens it; that task must not implement a competing project label. Report file or workspace construction failure without leaking a live slot or altering A. Shared-checkout warning does not authorize automatic isolation or suppress normal tool permissions.

**Deferred extensions (not acceptance criteria).** RecentProjects discovery and suggestions, a path-entry palette overlay, the grouped-panel N action, a searchable all-project history picker and cozyphi sessions list --all remain future candidates. This update does not erase the earlier proposal, implement it, or authorize that broader UI. Coordinate a future task only when selected; do not create duplicate tickets now.

**Blocked by:** [multisession-runtime-split](multisession-runtime-split.md), [multisession-registry](multisession-registry.md). Both are recorded done; this is the first ready delivery. No prerequisite on sessions-panel, hotkeys or the full prompt-snapshot refactor. [multisession-lifecycle-restore](multisession-lifecycle-restore.md) separately owns full child-work shutdown proof; this slice must still preserve existing safe close behavior.

## Acceptance Criteria

- /new <path> opens and selects a new project tab; /new without a path uses the active tab's project. Relative paths resolve from that project, ~ expands predictably, paths containing spaces work, and invalid/non-directory paths fail without disturbing live tabs or retaining a partial session slot.
- A and B use their own project instructions and declared workspace in system prompts, including fresh turns, child preparation, reconfiguration and compaction rebuilds; no global chdir or ambient process cwd selects the prompt project.
- B's controller, engine history cwd, project configuration and View agree. Filesystem/bash tools, local shell, watches, hooks context, memory/tasks and MCP/LSP identity follow their documented session/workspace/repository scope, with no A/B leakage.
- Opening an existing history selects its retained owner, constructs the correct workspace, or rejects a cwd mismatch before execution. A warning alone or merely removing cwdWarning is insufficient.
- The selector exposes project identity; opening another session in the same canonical checkout, including a symlink alias, warns without creating a worktree. Different worktrees are not collapsed solely by shared git root.
- Controlled two-project tests demonstrate simultaneous work, preserved drafts/transcripts, origin-bound asks and close of B without stopping A. History survives close; broad descendant-shutdown proof belongs to the linked lifecycle task.

## Verification Plan

1. Exercise the actual /new route with temporary A/B projects, distinct AGENTS.md content and same-named files; capture provider input through a fake transport and execute only controlled local tools.
2. Keep process cwd at A while testing B prompt construction/rebuild, child preparation, file/bash/local-shell/watch context and workspace-backed UI identity. Use adapters rather than live MCP/LSP servers or credentials.
3. Test relative and spaced paths, invalid directories, symlink-alias warning, shared-repository worktrees, failed assembly rollback and mismatched resume rejection/rebinding.
4. Run concurrent A/B work with a background ask, switch during delivery, close B and verify A plus both histories. Reuse retained-session tests; link remaining descendant-lifetime cases to lifecycle/hardening.
5. Format/build/test/race only changed packages; one scoped lint at most before commit. Update user-visible docs/CHANGELOG with the implementation, not with ledger maintenance.

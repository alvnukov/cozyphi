---
id: mcp-workspace-process-cwd
title: Start MCP stdio servers in their owning workspace
status: done
priority: high
model_level: high
task_type: bug
parent_id: multisession-runtime-split
tags:
    - mcp
    - workspace
    - isolation
branch: bug/mcp-workspace-process-cwd
worktree_path: .worktrees/mcp-workspace-process-cwd
acceptance_criteria:
    - MCP stdio processes start in their owning canonical workspace, not the application's ambient cwd.
    - Two workspace pools do not borrow each other's process configuration or cwd.
    - Existing callers retain compatibility and headless behavior remains fail closed.
verification_plan:
    - Controlled stdio helper reports cwd; two pools launched for different directories report their own workspace.
    - Run focused MCP tests and race checks.
    - Check all production LoadPool callers pass explicit workspace identity.
created_at: "2026-09-05T15:56:42.346495Z"
updated_at: "2026-09-05T16:47:35.596689Z"
---

## Body

Runtime-split audit found mcp.LoadPool selects a workspace config but internal/mcp/stdio.go constructs proc.Spec without a cwd, so a pool for another worktree starts servers in the application's ambient directory. Workspace identity must cover process launch, not only configuration cache keys. This is required before multiple workspace sessions can safely use shared runtime resources.

**Started (2026-09-05).** Prerequisite subtask of active multisession-runtime-split. MCP ownership changes are isolated within the parent's .worktrees/multisession-runtime-split worktree and will land with that verified runtime slice; no code edits in main.

**Done (2026-09-05).** Landed with runtime code commit 0d903f9 and local main integration. LoadPoolInDir pins canonical stdio cwd; Runtime/headless/MCP command assembly uses it. Pool/Client Close is permanent, including stale references. Real helper-process tests and full scoped MCP race tests passed; post-lint-fix MCP regressions passed. No separate worktree was created for this prerequisite; it was implemented within the parent runtime worktree. No push.

## Acceptance Criteria

- MCP stdio processes start in their owning canonical workspace, not the application's ambient cwd.
- Two workspace pools do not borrow each other's process configuration or cwd.
- Existing callers retain compatibility and headless behavior remains fail closed.

## Verification Plan

1. Controlled stdio helper reports cwd; two pools launched for different directories report their own workspace.
2. Run focused MCP tests and race checks.
3. Check all production LoadPool callers pass explicit workspace identity.

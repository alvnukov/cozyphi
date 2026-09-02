# Tasks

A repository can keep its work as a task registry: one markdown note per
task, kept by [mcp-ai-helper](https://github.com/alvnukov/mcp-ai-helper) and
tracked in git. The `task` tool lets the agent work that registry natively —
pick a task, start it, record progress, close it — against the same notes the
helper reads, without a server in between.

| Audience | This document |
| --- | --- |
| Users | Where the registry is, what the tool does, how a task moves |
| Contributors | The seams: note format, discovery, permission, registration |

---

## Where it lives

The registry is found from the **main checkout** of the repository, the way
the helper finds it:

1. `.mcp-ai-helper.yaml` at the root, when present, names it:
   `task_registry.obsidian.path` (default `obsidian-tasks`). A config that
   selects another backend (`lean`) is not a registry cozyphi can read.
2. Otherwise an `obsidian-tasks/` directory at the root is the registry.
3. Neither: the repository has no registry, and the tool is not offered.

The main checkout is the parent of Git's common directory, so a session
started in a linked worktree (`.worktrees/<id>`) works the same notes as one
started at the root. Sub-agents never carry the tool: a sub-agent is handed
one job, not the ledger of all of them.

## One task, one note

`<id>.md` is YAML frontmatter followed by three sections:

```markdown
---
id: fix-login-timeout
title: Fix the login timeout
status: todo
priority: high
model_level: medium
task_type: bug
parent_id: auth-epic
tags:
    - auth
branch: bug/fix-login-timeout
worktree_path: .worktrees/fix-login-timeout
acceptance_criteria:
    - login survives 5 minutes idle
verification_plan:
    - go test ./internal/auth/...
created_at: "2026-09-02T22:52:42.806862Z"
updated_at: "2026-09-03T08:10:11.120004Z"
---

## Body

Sessions drop after 30s of idle time.

**Started (2026-09-03).** taking it in .worktrees/fix-login-timeout

## Acceptance Criteria

- login survives 5 minutes idle

## Verification Plan

1. go test ./internal/auth/...
```

Statuses are `todo`, `in_progress`, `blocked`, `done`; priorities `critical`,
`high`, `medium`, `low`; model levels `very_high`, `high`, `medium`, `low`.
Ids are normalized the helper's way: lowercase, anything but letters, digits,
`.`, `_` and `-` collapsed to a dash. A `task_type` of `epic`, or the tag
`goal`, marks a container: it is never worked directly, its children are.

The format is the helper's, byte for byte: cozyphi writes with the same YAML
encoder and section layout, so a note that changes hands does not churn its
diff, and a note the helper would refuse is skipped with a diagnostic, never
rewritten. One consequence to know: the helper's reader treats every `## `
line as a section break, so a body may not contain one — text after it would
vanish. History goes into the body as dated bold labels instead, which is
what `start`, `done`, `block`, `reopen` and `note` write.

## Working a task

| Action | What it does |
| --- | --- |
| `current` | What to work on: ready tasks best first (in_progress, then by priority, then most recently touched), blocked ones apart. The default action. |
| `list` | Every task on one line; narrow with `status`, `type`, `tag`, `parent`. |
| `get` | One task in full. |
| `create` | A new note from `title` (+ `id`, `body`, `type`, `priority`, `model_level`, `parent`, `tags`, `acceptance_criteria`, `verification_plan`, `status`). |
| `update` | Change those fields; lists replace whole, an empty list clears. |
| `start` | `in_progress`, and the branch (`<type>/<id>`) and worktree (`.worktrees/<id>`) to work in, with the `git worktree add` line when the worktree is not there yet. Refuses a container. |
| `done` | `done`; `note` is required — what changed and where it landed. |
| `block` | `blocked`; `note` is required — what is in the way. |
| `reopen` | Back to `todo`. |
| `note` | A dated paragraph on the body, status unchanged. |

Every answer ends with a `Next:` line naming the natural next move for a
task in that state, and every mutation names the file it changed. The note is
a tracked file: it is committed with the work, in whatever way the repository
commits its ledger.

The system prompt carries one line about the tool, only when it is
registered: call `current` before choosing work, even when a task was named;
`start` what you take; close with `done` or `block` and a note.

## Permission

The gate sees two actions. `task_read` (`current`, `list`, `get`) is always
allowed. `task_write` (everything else) is allowed in interactive and
autopilot modes and refused in readonly mode — and so in plan mode, whose
tool set is readonly — because it is a mutation. There is no path for the
gate to vet: the registry directory is fixed at startup and a normalized id
cannot leave it. The plan gate exempts `task` from `plan_step`, like `memory`
and `watch`: bookkeeping is not a plan step.

## Seams

| Where | What |
| --- | --- |
| `internal/tasks` | `Registry`: `Discover`, `List`, `Get`, `Current`, `Create`, `Update`, `SetStatus`, `Note`; the note parser and renderer; `NormalizeID`, `BranchFor`, `WorktreeFor` |
| `internal/tools/tasktool` | The model-facing tool: argument parsing, the text of every answer, the `Next:` lines |
| `internal/permission` | `ActionTaskRead` / `ActionTaskWrite`, extracted from the `action` argument |
| `internal/agent` | `EngineOpts.Tasks`; the tool and the prompt line follow it |
| `internal/project` | `Project.RepoRoot()`, the main checkout the registry is discovered from |
| `cmd/run.go`, `internal/tui/controller` | Discovery at startup; a failure is a warning, not a refusal to start |

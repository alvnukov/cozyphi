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

A **target** is a checkout whose registry the tool can address. The default
is the **launch checkout** — the git root of the directory the session
started in — so a session inside `.worktrees/<id>` works that worktree's own
notes, and one at the root works the root's. When the launch checkout has no
registry but the main checkout does, the main checkout is the default. With
no registry anywhere the tool is not offered.

Each target's registry is found the helper's way: `.mcp-ai-helper.yaml` at
its root may name it (`task_registry.obsidian.path`, default
`obsidian-tasks`); otherwise an `obsidian-tasks/` directory at the root is
the registry. A checkout without either is not a target, and a config that
selects another backend (`lean`) is not a registry cozyphi can read.

The known targets are:

- `main` — the main checkout, the parent of Git's common directory;
- every **live worktree** `git worktree list` reports (prunable entries are
  not targets), labeled by its directory's base name — for a task worktree
  that base name is the task id;
- **external roots** the user vouched for in the global config
  (`~/.cozyphi/config.yaml`):

  ```yaml
  tasks:
    roots:
      docs: /Users/zol/src/project-docs
  ```

  A root entry must be named and absolute; a relative path or one stealing
  the `main` label is a config error at startup, not a silent skip. The names
  are the model's words for those registries.

Any action takes an optional `root` naming one of these labels. Reads default
to the launch checkout; **writes must name their root** — a write without a
label is refused with the labels that could have been said, so a checkout is
never dirtied by accident. The label set is resolved on every call: a
worktree created mid-session is a target on the next call, no restart. A miss
is visible, not silent: answers name the file they touched by absolute path
when it sits outside the current directory.

Sub-agents never carry the tool: a sub-agent is handed one job, not the
ledger of all of them.

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
task in that state, and every mutation names the file it changed. A write's
`Next:` line spells the calls with `root=<label>`. The note is a tracked
file: it is committed with the work, in whatever way the repository commits
its ledger.

The system prompt carries a paragraph about the tool, and it varies with the
permission level. At `write` and `ask` it is the workflow: call `current`
before choosing work, even when a task was named; `start` what you take;
close with `done` or `block` and a note — `ask` adds that every write asks
the user first, so make one complete change rather than several small ones.
At `read` it says the registry can be read but not changed, and asks for a
change to be described for the user to make. At `off` there is no paragraph,
because there is no tool.

## Permission

`permissions.tasks` in the global config (`~/.cozyphi/config.yaml`) decides
how far the model may go with the registry:

```yaml
permissions:
  tasks: ask   # off | read | ask | write (default)
```

Absent or empty means `write`. The levels, in order of trust:

| Level | Tool | Prompt | Gate |
| --- | --- | --- | --- |
| `off` | not registered at all, even when the repository has a registry | no paragraph about the registry | denies `task_read` and `task_write` |
| `read` | registered with only `current`, `list`, `get` in the schema; the description says the registry is read-only and asks for a change to be described for the user to make | the registry can be read but not changed; describe changes for the user | denies `task_write`, naming `permissions.tasks: read` and asking for the change to be described for the user |
| `ask` | every action | the workflow, plus: every write asks the user first, so make one complete change rather than several small ones | asks on every `task_write`; the user answers each one in the TUI |
| `write` | every action | the workflow | allows |

The level applies the same in every mode, plan mode included. A task note is
bookkeeping about the work, not the work itself, and shaping tasks is part of
planning — so `task_write` is no longer a mutation to the readonly fold.

Two mode-shaped consequences remain. Under `ask` the question reaches the
user in readonly and plan mode too: readonly normally folds an ask into a
denial, and `task_write` is the one exception, because the user who chose to
be asked is there to answer. In `autopilot` and `headless-strict` nobody can
answer, so an ask folds to a refusal and `ask` behaves like `read`.

In plan mode at a writable level (`ask` or `write`) the plan-mode appendix of
the system prompt adds a rule: the registry is planning material, not a
change. `create`, `update`, `note`, `block` and `reopen` are allowed in the
plan; `start` waits for execution.

The General settings tab carries a row, `Task registry access: <level>`. A
click steps it write → ask → read → off → write. Ctrl+S saves it to
`permissions.tasks`, leaving the rest of the `permissions:` section as the
user wrote it, and applies it live: the gate uses the new level at once, the
tool list and the prompt paragraph follow at the next model round.

An unknown value (`tasks: maybe`) is a config error naming the four choices,
both when a session starts and when the settings pane opens.

There is still no path for the gate to vet: a normalized id cannot leave
its target's registry directory, and the label a call names resolves to one
of the targets above — an unknown one is refused by the tool with the known
labels, before anything is read. The gate's two actions stay `task_read` and
`task_write`. The plan gate exempts `task` from `plan_step`, like `memory`
and `watch`: bookkeeping is not a plan step.

## Seams

| Where | What |
| --- | --- |
| `internal/tasks` | `Registry`: `Discover`, `List`, `Get`, `Current`, `Create`, `Update`, `SetStatus`, `Note`; `DiscoverTargets` / `Targets`, the label set and its per-call resolution; the note parser and renderer; `NormalizeID`, `BranchFor`, `WorktreeFor` |
| `internal/tools/tasktool` | The model-facing tool: argument parsing, the `root` label, the text of every answer, the `Next:` lines |
| `internal/permission` | `ActionTaskRead` / `ActionTaskWrite`, extracted from the `action` argument |
| `internal/agent` | `EngineOpts.Tasks`; the tool and the prompt paragraph follow it |
| `internal/project` | `Project.CheckoutRoot()`, the launch checkout; `Project.RepoRoot()`, the main checkout; `Tasks.Roots` from the global config |
| `cmd/run.go`, `internal/tui/controller` | Target discovery at startup; a failure is a warning, not a refusal to start |

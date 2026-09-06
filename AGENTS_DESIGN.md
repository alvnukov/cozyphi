# Sub-agent UX design

Reference behaviour for sub-agents in the cozyphi terminal UI, and the gap
between it and what the current code does. The reference is Claude Code as
documented on 2026-09-06 (v2.1.23x); the gap analysis is against `main` at
391f089. Delivery is tracked in `obsidian-tasks/subagent-panel-ux.md`.

Sources (checked, not remembered):

- https://code.claude.com/docs/en/sub-agents — "Run subagents in foreground or
  background", "Observe and steer running forks", "Resume subagents", "Fork
  the current conversation"
- https://code.claude.com/docs/en/interactive-mode — `Ctrl+B`, `Ctrl+T` vs
  `/tasks`, `Ctrl+X Ctrl+K`
- https://code.claude.com/docs/en/agents — subagents vs agent view vs teams
- https://code.claude.com/docs/en/fullscreen — `Ctrl+O` transcript mode

## How Claude Code presents sub-agents

**Surfaces.** A sub-agent is never a top-level tab. It appears in exactly three
places: a tool-call row in the main transcript, a row in the panel below the
prompt input while it runs, and the `/tasks` list afterwards.

**Transcript row.** `subagent-name(short task description)`. While running it
is a live tool row; on completion it shows tool-call count, tokens and elapsed
time. `Ctrl+O` on the row expands the full details. Only the final result
enters the main context; the sub-agent's own tool calls stay out of it.

**Panel below the prompt.** Shown while sub-agents or forks run: one row for
`main` and one per running sub-agent, nested sub-agents as a tree, a row whose
descendants are hidden carries `(+N)`. Keys: `↑`/`↓` move, `Enter` opens the
row's transcript in a detail pane, `Esc` closes the pane or collapses a node,
`→`/`←` expand/collapse, `Space` pauses/resumes a fork, `x` stops it. Typing
into an open transcript resumes that sub-agent (v2.1.191+).

**Row lifecycle.** A sub-agent that finishes successfully leaves the panel at
once; the footer shows `/tasks to see subagents` for 30 seconds. A sub-agent
that fails or is stopped keeps its row for 30 seconds; `x` clears it sooner.

**`/tasks`.** Lists the session's background work, including finished
sub-agents: done rows sorted below running ones, kept for the same 30-second
window. `Enter` opens the transcript, `x` stops. `Ctrl+T` is the todo checklist
and is unrelated; `/agents` no longer opens a panel (v2.1.198+).

**Background by default.** In an interactive session every sub-agent runs in
the background and the model cannot ask for the foreground. Foreground exists
only with `CLAUDE_CODE_DISABLE_BACKGROUND_TASKS=1` or in `-p`/SDK runs.
`Ctrl+B` moves a foreground task to the background; `Ctrl+X Ctrl+K` (twice)
stops every background sub-agent.

**Permissions.** A background sub-agent's permission prompt surfaces in the
main session and names the sub-agent. Approve lets it continue; `Esc` denies
that one tool call without stopping the sub-agent. A session-wide grant given
there applies to the main conversation too.

**Results.** The result reaches the model as a completion notification in a
later turn; the model waits for it before reporting. For the user it "arrives
as a message in your main conversation".

**Resume.** `SendMessage` to a finished or model-stopped sub-agent resumes it
in the background under the same id, and it shows as running again. A
sub-agent the user stopped with `x` refuses messages. Transcripts persist at
`~/.claude/projects/{project}/{sessionId}/subagents/agent-{agentId}.jsonl`.

**Limits.** 20 concurrent (`CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS`); background
sub-agents get a reduced built-in tool set.

**Not sub-agents.** `/fork` copies the whole session into a background session
shown in agent view (`claude agents`); `/subtask <task>` starts a fork
sub-agent that inherits the conversation and lands in the same panel.

## What cozyphi does today

Interactive children are on by default (`cmd/main.go` enables them). Each
terminal `agent_spawn` creates a retained child View through
`interactiveRunner` in `internal/tui/controller/children.go`.

1. **Children are tabs.** Every child takes a slot in the session selector
   (`○ name [running]`, cap 12 retained), stays after completion and needs
   `/close` or `×`.
2. **The parent's spawn row is empty.** `interactiveRunner.Run` never calls
   `env.OnProgress`; `EngineRunner.Run` is the only caller. The transcript
   `SubagentStore` therefore receives no child tool rows, no count, no elapsed
   time. The row is titled with the raw tool name `agent_spawn`.
3. **The outcome is invisible.** `Session.AcceptOutcome` appends a
   `<system-reminder>` user message; no session event or bus message follows,
   so the parent transcript shows nothing live, and on resume
   `memory.StripReminders` empties it.
4. **Attention pulls the user away.** Child asks and errors set `unread:N`
   and a notice line `#N name: attention — /switch N`, sending the user into
   the child tab.
5. **No list.** Footer says `N jobs`, selector shows `⚙N`; there is no
   `/tasks`; a child can be stopped only through the model's `agent_cancel`.

## Target for cozyphi

Keep the child as a retained interactive session (that is the "detail pane":
`Enter` opens it, typing steers it). Change only how it is presented.

1. **Transcript row** `role(description)` instead of `agent_spawn`; live
   `· N tools · 1m20s` fed by `job.Progress` from the interactive runner;
   on completion the counts plus the outcome summary under the row;
   `Enter`/click expands the child's tool tree.
2. **Panel under the composer**, shown only while children live: `● main`,
   then `○ role(description) · N tools · 1m20s` per child, nested as a tree
   with `(+N)`. `↑`/`↓`, `Enter` opens the child View (no tab), `Esc` returns
   to the parent, `x` stops, `→`/`←` expand/collapse. Entry via `/tasks` or a
   hotkey from `multisession-hotkeys`; the composer keeps `Enter`.
3. **Row lifecycle** as in Claude Code: success clears the row at once and the
   footer shows `/tasks — сабагенты` for 30 s; failure or stop keeps the row
   30 s, `x` clears it. Timers use an injectable clock.
4. **`/tasks`** as a list pane on the `watchpane` pattern: running on top,
   done below; `Enter` opens the transcript, `x` stops, `Esc` closes.
5. **Outcome as a transcript row** in the parent, rendered like `RoleWatch`
   and `RoleNotice` local rows, so it is visible live and after resume. The
   model still receives the same `<system-reminder>`.
6. **Child permission asks in the parent**, prefixed with the child's name,
   while the child View is not open; `Esc` denies only that call and the child
   continues; a session-wide grant applies to main. An open child View keeps
   its asks locally, as now.
7. **Selector** shows no children; child attention goes to the panel row and
   the footer, never to a `/switch N` notice.

Out of scope: `Ctrl+B` (every child is already background), `Space` pause,
the 20-concurrency limit (cozyphi keeps 12 retained), forks and `/subtask`,
the deferred grouped sidebar (`multisession-sessions-panel`).

Invariants that stay: transcripts under `~/.cozyphi/jobs/<id>/`; the parent
model gets the wait/outcome summary only; child engines carry no `agent_*`
tools; default child role is explore; outcomes are untrusted child data and
never approve anything.

## Delivery

Four phases, each its own branch and worktree
(`obsidian-tasks/subagent-panel-ux.md`):

- A `subagent-row-progress` — items 1 and 5.
- B `subagent-panel` — items 2, 3 and 7.
- C `subagent-tasks-pane` — item 4.
- D `subagent-ask-routing` — item 6.

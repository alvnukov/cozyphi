# Sub-agent UX design

Reference behaviour for sub-agents in the cozyphi terminal UI, the gap
between it and the current code, and the agreed target. The reference is
Claude Code as documented on 2026-09-06 (v2.1.23x); the gap analysis is
against `main` at 391f089. Delivery is tracked in
`obsidian-tasks/subagent-panel-ux.md`.

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
   (`○ name [running]`, cap 12 retained, shared with the user's own
   sessions), stays after completion and needs `/close` or `×`.
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
5. **No list.** Footer says `N jobs`, selector shows `⚙N`; there is no list
   command; a child can be stopped only through the model's `agent_cancel`.

## What we take from Claude Code, what stays cozy

Agreed with the user on 2026-09-06.

**Taken from Claude Code** (the surface):

- Children are never tabs. A scrollable panel below the composer shows the
  live ones; it disappears when empty.
- The parent transcript row is alive (`role(description) · N tools · 1m20s`)
  and the outcome lands in that same row.
- A list command with history and `Enter` open / `x` stop / `Esc` close.
- A child's permission ask is answered from the parent, prefixed with the
  child's name; `Esc` denies only that call and the child keeps running.
- A footer hint after completion instead of an attention notice that drags
  the user into another tab.

**Kept cozy** (deeper than Claude Code):

- A child is a full interactive session: model, effort, one-turn interrupt,
  input queue, plan. `Enter` on its row opens that session as the current
  screen; the panel's `main` row leads back. Claude Code's detail pane is a
  transcript plus resume.
- Roles and skills stay in the row: `explore(description) · skills: a, b`,
  `no skills: reason`, a pinned model. Claude Code shows only the agent type.
- No nesting (invariant: child engines carry no `agent_*` tools), so the
  panel is flat: no tree, no `(+N)`.
- Jobs are durable on disk (`~/.cozyphi/jobs/<id>/` with `meta.json` and
  `result.md`), so the list keeps the whole session's history, not a
  30-second window. The panel still clears at once on success because the
  transcript row already shows the result.
- Outcome delivery follows the watches pattern: a local transcript row for
  the user and a reminder block for the model, never a user message.
- Grants given for a child's ask apply to that child within its role ceiling;
  the parent is untouched. Claude Code's session-wide leak into main is not
  copied.
- Explicit `agent_wait` as a barrier and headless mode stay. `Ctrl+B` and
  foreground runs are unnecessary: every child is already background.
- The session selector stays for the user's own sessions; the parent tab
  keeps its `⚙N` live-children mark.
- No token counts in rows: the job records none, and nothing is invented.

**Decided:** the list command is `/agents` (`tasks` already means the task
ledger in cozyphi); no promote-to-tab in this version (only later, and only
if it can be made beautiful); asks surface as an overlay in the parent; one
row per child.

## Target contract

1. **Transcript row.** Title `role(description)` with the existing
   ` · skills: …` / ` · no skills: …` and ` · <model>` suffixes; live
   ` · N tools · 1m20s` fed by `job.Progress` (the interactive runner must
   emit `OnProgress` like `EngineRunner`); elapsed ticks through the draw
   loop's `WakeIn` while running, never a timer of its own. On completion the
   glyph settles (`✓`, `✗`, `■` stopped), counts freeze and the outcome
   summary renders inside the block. `Enter`/click expands the child's tool
   tree, as today.
2. **Outcome in the parent.** The delivered outcome updates the same block,
   live and after resume: replay recognises the outcome delivery message by
   its `DeliveryID` (`…:terminal`) and feeds the store instead of stripping
   it. When the spawn row is gone (compaction), a local row
   `agent outcome · role(description)` carries the summary. The model still
   receives the same `<system-reminder>`.
3. **Panel below the composer.** Rendered between the composer and the
   footer whenever the session has children, and always while a child's own
   screen is up: a first row `main` for the parent, then one row per running
   child of the session, plus failed or stopped children inside their
   30-second window. The screen decides the panel — the child on screen keeps
   its row whatever state it reached, so the `main` row that leads back can
   never disappear from under the user. The row of the current
   screen wears `●`, the others `○`. A running child's row reads
   `⟳ role(description)  <what it is doing>` — the second field is the child's
   latest tool call, worded exactly as that call's own row is titled in the
   transcript, and absent until the child has made one. Nothing else rides the
   row: the tool count, the elapsed time, the skills and the model stay in the
   parent's transcript row and in `/agents`, where there is width for them. The
   other states say how they stand instead: `⏸ … · waiting: permission`,
   `✗ … · failed`, `■ … · stopped`. Viewport of at most three rows; more
   rows scroll under the DESIGN.md motion dialect (arrows, `j`/`k`, wheel),
   with unselectable indicator rows `↑ N more` / `↓ N more` whenever rows are
   hidden above or below. The selected row is marked by a `❯` cursor in a
   two-cell column every band line starts with — the reference's own way of
   showing it — and never by a reversed fill across the width. While the panel
   holds the keyboard it wears the agents scope's own hint row from the keys
   catalog directly above it, and loses that row when focus goes back to the
   composer. No close button: rows
   leave on their own. Focus moves from the composer into the panel with `↓` when
   the cursor is on the composer's last line (or the composer is empty), and
   back with `↑` on the `main` row; `Esc` also returns to the composer. Keys
   inside: `Enter` on `main` shows the parent, `Enter` on a child opens that
   child session as the current screen, `x` stops a running child or clears
   a failed/stopped row. Mouse click selects and opens. The composer keeps
   `Enter`.
4. **Inside a child.** The child View is the current screen with no selector
   tab. The same panel stays under its composer with the child's row marked
   `●`, so `↓`, `Enter` on `main` (or a click on it) leads back to the
   parent, and siblings are one row away. `Ctrl+]` (the keys catalog's
   `agent-back`) leads back to the parent from the composer, running child or
   finished one alike. `Esc` there means what it means in every session —
   recall a queued prompt, then stop the run — and never leaves the screen;
   `x` in the panel and `Ctrl+C` stop a child too. The selector marks the owning tab ` › role(description)` and the
   footer carries the way back, so the user always knows whose screen this is.
5. **Row lifecycle.** Success clears the panel row at once and the footer
   shows `/agents to see agents` for 30 s. Failure or stop keeps the row 30 s;
   `x` clears it. Neither rule touches the row of the child on screen: it is
   kept, its footer hint is armed only once it leaves, and its failure window
   is counted from when the run ended, not from when the user left. A row also
   follows the live child over the parent's recorded outcome, so a follow-up
   reads `⟳` again instead of staying frozen on the first result. Timers come
   from an injectable clock so tests can drive them.
6. **`/agents`.** A list pane on the `watchpane` pattern: running children
   on top, finished below (whole session history from the job manager), each
   with status, tools, elapsed and a one-line summary. `Enter` opens the
   child session, `x` stops, `Esc` closes. Footer `N jobs` becomes
   `N agents`.
7. **Child asks in the parent.** While the child View is not the current
   screen, its permission, question and continue asks appear in the parent's
   overlay prefixed `[role(description)]`. Approve lets the call run; `Esc`
   denies only that call; scope choices bind to the child session and its
   role ceiling. When the child View is the current screen, its asks stay
   there as now. Child asks mark the panel row `waiting` and raise the
   parent's attention; child completion raises no OS notification (the
   parent's wake and turn end do).
8. **Selector and caps.** Children never appear in the selector. They no
   longer consume the 12 retained user-session slots: retained children get
   their own cap of 12 with the oldest finished child released first, and
   `job.Manager.MaxConcurrent` bounds running ones.

Out of scope: `Ctrl+B`, `Space` pause, tree/`(+N)`, tokens, forks and
`/subtask`, a model-side resume tool (separate ticket; the runtime already
supports linked follow-ups for user input), merging watches into `/agents`,
promote-to-tab, the deferred grouped sidebar (`multisession-sessions-panel`).

Invariants that stay: transcripts under `~/.cozyphi/jobs/<id>/`; the parent
model gets the wait/outcome summary only; child engines carry no `agent_*`
tools; default child role is explore; outcomes are untrusted child data and
never approve anything.

## Delivery

Four phases, each its own branch and worktree
(`obsidian-tasks/subagent-panel-ux.md`):

- A `subagent-row-progress` — items 1 and 2.
- B `subagent-panel-widget` (the `internal/tui/agentpanel` widget) and
  `subagent-panel` (its wiring: `sessions.Family`, child screens) — items 3,
  4, 5 and 8.
- C `subagent-agents-pane` — item 6 (`internal/tui/agentlist`).
- D `subagent-ask-routing` — item 7.

All four phases merged into `main` on 2026-09-06. The "What cozyphi does
today" section above describes `main` at 391f089, before this work, and is
kept as the record of the gap that was closed. Two decisions taken during
delivery: all UI text is English (indicator rows `↑ N more`, footer hint
`/agents to see agents`); an ask belongs to the family rather than to a
screen, so it follows the user from the parent's screen to a child's and back,
unanswered and never duplicated, and a second ask waits in the family's queue
while the first is on screen, its session's panel row reading
`⏸ … waiting: permission`.

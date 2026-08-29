# Plan Actions and Per-Step Models

## Problem Statement

The durable plan tracks work, but it is passive. The agent must remember to do
routine per-step housework itself — compact the context before a heavy phase,
read the skill files a step depends on — and each of those costs a turn,
context, and vigilance. Model choice is the same story at a different scale:
the session has exactly one model, so cheap exploration runs on the same
(expensive) model as difficult edits, and there is no way to plan that
division of labor ahead of time. The user cannot see, let alone steer, any of
this from the sidebar without leaving the plan.

## Solution

Plans gain **actions** and **models**.

Actions are built-in commands attached to plan lifecycle events
(`step_start`, `step_end`, `plan_start` — fired when the plan is approved —
and `plan_end`, fired when the plan closes). The harness runs them
automatically, synchronously before the transition is recorded; a failed
action rejects the transition so the plan never lies about what happened.
Every run is visible as a transcript row and as a status line in the sidebar
plan pane.

Models become plan data at two levels: a per-step-type model map in plan
settings (explore on a cheap model, edit on a strong one), and an optional
per-step override. The effective model is applied at step start and reverts
to the session model when the plan closes. The user can pick a step's model
directly in the sidebar (overlay picker on the selected step) or edit the
full map in the plan editor's new settings section.

## User Stories

1. As a user, I want to attach a context compaction action to the end of a
   heavy step, so that context stays small without the agent spending a turn
   on it.
2. As a user, I want to attach a skill-injection action to the start of a
   step, so that the step's first turn is already aware of the skill it
   needs.
3. As a user, I want plan-level actions on approval and on close, so that
   recurring plan-wide housework (e.g. compaction) is automated too.
4. As a user, I want actions shown as a compact chip line under their step
   in the sidebar, so that I can see what automation will run without
   opening anything.
5. As a user, I want plan-level actions shown under the plan header, so plan
   and step automation are visually distinct.
6. As a user, I want each action run to produce a transcript row with its
   result, so that all automatic activity is auditable.
7. As a user, I want a failed action to block its transition with an
   actionable error, so the plan never records a step as done when its
   automation did not happen.
8. As a user, I want a failed close-time action to reject the whole closing
   transition (all-or-nothing), so plan closure is not partial.
9. As a user, I want actions to re-run on repeated transitions (reopen →
   complete again), so automation follows the real lifecycle, not the
   intended one.
10. As a user, I want automation to exist only in approved plans, so an
    unapproved draft never executes anything.
11. As an agent, I want to author actions through the plan tool when
    creating or patching the plan, so automation is part of planning itself.
12. As a user, I want to add, remove, and edit actions (event, type,
    parameter) in the plan editor, so I can correct what the agent proposed.
13. As a user, I want unknown model names and unknown skills in a plan-tool
    patch to be rejected with the list of valid options, so typos never
    enter the plan silently.
14. As a user, I want a per-step-type model map in plan settings, so cheap
    work runs on a cheap model without per-step configuration.
15. As a user, I want to override the model of a specific step, so one
    unusually hard step can use a stronger model than its type.
16. As a user, I want to select a step in the sidebar (click or keyboard
    cursor) and open an overlay model picker for it, so I can steer the
    model without leaving the sidebar.
17. As a user, I want a "by step type" entry in that picker, so clearing an
    override is as easy as setting one.
18. As a user, I want sidebar picks applied as an immediate patch with the
    reapproval banner when the plan was approved, so I always know a
    material change reset approval.
19. As a user, I want the effective model precedence to be step override →
    step type → session default, so I can reason about one rule.
20. As a user, I want the step's model badge shown on the step line only
    when an override exists, so the sidebar stays quiet by default.
21. As a user, I want the session model restored after the plan closes, so
    plans do not leak their model choices into ordinary chat.
22. As a user, I want a step blocked with a clear reason when its configured
    model disappeared from the config, so breakage is loud and repairable.
23. As a user, I want sub-agents spawned during a step to inherit that
    step's model, so the whole step's work shares its economics.
24. As a user, I want actions, type models, overrides, and last run statuses
    to survive a session restart, so a resumed plan keeps its automation and
    its history.
25. As a user with a legacy plan, I want no actions and no models to behave
    exactly as today, so the feature costs nothing when unused.

## Implementation Decisions

- **Plan schema**: a step carries an actions list and an optional model
  override; the plan carries an actions list and a per-step-type model map.
  An action is `{event, type, params}` with event
  `step_start|step_end|plan_start|plan_end` (per-level) and an extensible
  built-in type enum; v1 ships `compact` (no params) and `inject_skill`
  (skill name list).
- **Built-ins only**: no shell-command actions in v1. If arbitrary commands
  are added later, they enter through plan approval as a visible whitelist,
  never silently.
- **Run records**: every action run appends a record (status, error,
  timestamp) to plan state; the sidebar shows the last one. Runs are kept,
  not overwritten, matching the append-only session log.
- **Execution semantics**: actions run synchronously before the transition's
  durable write. Any failure rejects the transition — the step stays where
  it was and the agent receives a corrective error naming the action. The
  close transition runs the step's `step_end` and the plan's `plan_end`
  actions as one all-or-nothing batch.
- **Event timing**: `plan_start` fires at approval; `step_start`/`step_end`
  fire on the existing step transitions; `plan_end` fires on close
  (success and abandoned alike). Actions execute only in approved plans.
- **`inject_skill`** reuses the existing skill read-instruction mechanism:
  the step's first turn is told to read the named skills; bodies load lazily
  on demand. **`compact`** reuses the existing compaction engine, invoked
  synchronously at the event.
- **Model resolution**: step override → step-type model → session default,
  resolved and applied at step start through the existing engine model swap
  (round-boundary, no session switch). After the plan closes, the session
  model is restored. Sub-agent engines already snapshot the parent's current
  model at spawn, which yields inheritance for free.
- **Validation**: plan-tool authoring is fail-closed — unknown model names,
  unknown skills, unknown events or types reject the patch with the valid
  options. UI pickers are list-only. A model that disappears from the config
  after authoring blocks the step at start with the reason; it does not
  silently substitute.
- **Materiality**: actions, the type model map, and step overrides are
  material fields — changing them in an approved plan resets approval and
  shows in the existing reapproval diff banner.
- **Sidebar**: an action chip line under each step (icon + name, status by
  marker color: waiting/ok/failed) and plan-level actions under the plan
  header. Steps gain a selection cursor (mouse click on a row; ↑↓ while the
  plan pane is focused) and `m`/Enter opens an overlay model picker listing
  configured and provider models plus a "by step type" entry; Esc cancels.
  A pick patches immediately, like the approval checkbox.
- **Plan editor**: a new settings section edits the per-step-type model map
  with the same picker; step detail gains the override picker and full
  action editing (add/remove/change event, type, params).
- **Transcript**: one row per action run, success or failure, naming the
  action and its result.
- **Plan tool**: create/patch accept the new fields; the tool description
  documents the semantics above so the model can plan with them.

## Testing Decisions

- A good test observes external behavior — session log entries, observable
  engine state, rendered sidebar rows — never internals.
- Three existing seams, no new interfaces:
  1. **Session state machine**: schema validation (fail-closed authoring),
     material diff including the new fields, transition gating on action
     failure, persistence round-trip through the session log. Prior art: the
     existing plan transition, patch, and diff tests.
  2. **Agent engine**: through its public methods with a stub LLM client —
     actions execute on transitions, failure blocks, `inject_skill` adds the
     read-instruction to the step's first turn, the model is applied at step
     start and restored at plan close. Prior art: the existing engine
     session and resume-model tests.
  3. **TUI widgets**: sidebar and plan editor render from `session.Plan`
     snapshots — chip lines, badges, cursor, picker overlay, settings
     section — plus key and mouse handling. Prior art: the existing sidebar
     and plan editor tests.

## Out of Scope

- Shell-command actions (future: admitted through plan approval).
- Plugin hook events for plan transitions.
- Events beyond the four (block/resume/cancel/reopen stay non-automatic).
- Editing actions or type models from the sidebar (the sidebar edits only a
  step's model override).
- Sticky model semantics (superseded: the model is a function of the
  current step), per-action retries or backoff, action parameter templating,
  and per-role sub-agent model overrides.

## Further Notes

- Superseded decisions, kept for the record: per-step model only → per-type
  map + per-step override; sticky model → per-step-type resolution; sidebar
  picker editing type models → picker editing a single step's override.
- Design stance: automation must be visible (sidebar, transcript) and fail
  loudly (blocked transitions) — a plan that hides what it did, or records
  success over a failed action, is worse than no automation.
- Implementation must add a `## [Unreleased]` line to `CHANGELOG.md` (repo
  rule for user-visible changes).

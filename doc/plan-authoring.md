# Plan authoring

## authoring_policy

`plan.defaults.authoring_policy` in `~/.cozyphi/config.yaml` selects the
authoring grammar the plan-mode prompt carries. It is a closed selector
with two values; anything else fails config load.

| value | plan-mode prompt |
| --- | --- |
| `adaptive-minimal` (default; also when the key is absent) | appends the authoring grammar: obligations over workstreams, dependency and uncertainty naming, evidence bounds, the smallest complete plan, the least sufficient capability type, and a model-side self-check |
| `legacy` | the pre-grammar appendix, byte-identical |

The selector changes prompt text only. It never alters permissions, the
plan gate, approvals, or plan lifecycle — those live in the step types and
exemptions of the same section.

The Settings modal exposes the same choice on the *Plan defaults* tab
(*Authoring grammar* row); Apply persists it into `config.yaml`.

## Execution model and effort

The planner can set a step's optional `effort` field on create, legacy update,
and patch (including inserted and superseding steps). Values are `none`,
`minimal`, `low`, `medium`, `high`, `xhigh`, and `max`. Choose the least sufficient
reasoning depth; this ladder does not imply that every model supports every level.
Invalid values fail before saving; unsupported levels fail before start effects.

Omitting effort on a new step inherits the user-selected model's effort. In
`update_step`, omission preserves the current override; `effort: null` or
`effort: ""` clears it. Effort changes are material and require reapproval.

Model identity is human-only. The tool neither advertises a model catalog nor
accepts actionable `model` fields, including empty or null values. User step
pins, type defaults, session configuration, and persisted `name:effort` references
remain supported. Resolution is step pin → type default → original session model,
then the independent effort override. Effort-only steps also save and restore the
original session configuration, so one step cannot inherit another's temporary pin.
Provider options are not rewritten; their existing precedence remains unchanged.
Model settings stay out of model-facing plan views and diffs; the user UI retains
the canonical settings. Approval and automatic actions remain user-owned.

## Tool availability

The provider keeps the session's tool schemas across approval, step and mode
changes. Schemas describe capabilities; they do not grant execution. Missing
managers and explicit child/custom tool sets still limit which schemas exist.
In `useplan`, the existing plan gate checks each call before permission checks
or dispatch. In `plan`, write/edit schemas remain visible but their handlers
remain absent from the executor.

A stable catalog is also what keeps the provider's prompt cache warm: tool
schemas are the first segment of every request, so a list that changed on
approval or on a step transition invalidated the cached system prompt and
the whole conversation behind it.

A phase refusal reaches the model as JSON under `tool_error`, carrying
`code`, `tool`, `current_phase` (`plan` or `useplan`), `reason`,
`next_action` and `retry_policy`. The code names the one recovery that
unblocks the call, so the model need not parse the prose:

| `code` | Cause | Recovery |
| --- | --- | --- |
| `TOOL_REQUIRES_PLAN` | no plan draft exists | create the smallest plan, wait for approval |
| `TOOL_REQUIRES_APPROVAL` | a draft exists, not yet approved | tell the user, wait for approval |
| `TOOL_REQUIRES_STEP` | no startable step of a compatible type is bound | bind the same tool to a compatible pending/in_progress step |
| `TOOL_REQUIRES_PLAN_REPAIR` | the approved plan has a step of an unconfigured type | repair the plan, wait for re-approval |
| `TOOL_FORBIDDEN_IN_PHASE` | `plan` mode withholds the handler | finish the read-only plan; only the user switching mode unblocks it |
The same blocked action must not be attempted through a different tool,
shell command, script or delegation. The transcript keeps its existing plain
reason so approval-resume handling remains compatible. Hint mode continues
to execute with advisory feedback; skill-preload retries are unchanged.

## Step skills

A plan step's enabled skills are runtime context resources, not file-read tasks.
At step start the engine resolves them in plan order, removes duplicate names,
and injects each complete `SKILL.md` as plain text before working tools run.
Disabled skills are skipped. If the tool call itself caused the step transition,
the engine installs the context and refuses that call once with retry guidance;
the repeated call then runs with the selected guidance already present.

## Telemetry

Authoring friction is observable through `internal/plantel` counters only:
drafts created (tagged by this selector), approval latency buckets, material
reapprovals, patch retries and completion outcomes. The privacy boundary is
explicit: the snapshot is a fixed set of `uint64` fields — no plan text, step
text, prompts, tool output or repository content ever enters telemetry, no
label is free-form, and nothing recorded there feeds back into authoring
decisions. The numbers are read-only, for humans and dashboards. Each counter
fires at its production call site: draft creation, the first decisive approval
grant (a material reset since adds a reapproval), a stale-revision patch
rejection, and the final close.

## Scenario gate

`internal/planscen` is the integration gate for this increment: ten
deterministic scenarios — trivial task, uncertain bug, compound work,
read-only run, novel no-match, risky JIT step, custom type names, stale hint,
unavailable tool, and mid-plan material adaptation — each walked through the
*real* plangate policy and the *real* session lifecycle: strict v2 contract
validation, durable replace, user approval, the permission gate itself, patch
and supersede, transitions, and the final close. No permission-gate mocks. The
mid-plan scenario is the convergence claim: supersede (never cancel) adapts an
approved plan mid-flight, approval resets, the user re-approves, and the plan
closes as success with the superseded step retired but its evidence kept.

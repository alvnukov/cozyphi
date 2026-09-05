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

The planner can set a step's optional `model` field when creating or patching a
plan. Its tool schema lists executable model names and their supported
`name:effort` combinations, for example `configured-model:low`. It should choose
the least expensive sufficient model and effort for the step's complexity;
levels come from that model's catalog, not a universal ladder.

Omitting the field inherits the plan's type default or the session model.
A patch can clear a pin with `model: null`. Unknown models and unsupported effort
combinations are rejected before saving, and the executor rechecks the model
when the step starts. Planner-authored pins remain visible in plan responses.
Approval, automatic actions and type defaults remain under the user's control.

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

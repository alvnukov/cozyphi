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

## Telemetry

Authoring friction is observable through `internal/plantel` counters only:
drafts created (tagged by this selector), approval latency buckets, material
reapprovals, patch retries and completion outcomes. The privacy boundary is
explicit: the snapshot is a fixed set of `uint64` fields — no plan text, step
text, prompts, tool output or repository content ever enters telemetry, no
label is free-form, and nothing recorded there feeds back into authoring
decisions. The numbers are read-only, for humans and dashboards.

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

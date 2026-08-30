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

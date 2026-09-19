---
id: web-research-25-post-web-grants
title: 25 — Require narrow effective-action grants after checked web evidence
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - A passed web answer remains untrusted for subsequent action and egress admission.
    - Narrow grants bind actual action, data/version, final arguments, recipient, expiry and uses; mismatches and exhausted grants do not execute.
    - Web restrictions survive off/observe/enforce, ordinary allow-all and permission-bypass flags; legitimate matching granted work still succeeds.
verification_plan:
    - After a passed source, invoke controlled mutation and egress sinks with no grant, matching grant, changed arguments/data/recipient, expired grant and exhausted uses.
    - Repeat off/observe/enforce and normal allow-all; observe sink effects, not only permission return values.
    - Attach T31 matrix and scoped public-executor tests.
created_at: "2026-09-19T19:17:18.835676Z"
updated_at: "2026-09-19T19:17:18.835676Z"
---

## Body

**What to build:** Integrate web-derived material into the shared action gate so legitimate work is possible without blanket trust.

**Blocked by:** [24](web-research-24-durable-lineage.md), [dataflow policy](enforce-agent-dataflow-policy.md).

**Contract:** [Spec](../specs/protected-web-research.md), D7/D8/D16, Q1/Q14/Q25/Q32, T02/T20/T31. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Exercise final effective arguments through existing shared gates for a representative mutation and egress action after checked web evidence. Preserve grants independent of normal session allowlists. Require renewed authority after data/recipient/action change, expiry or use exhaustion. Route uncertainty to safe human interaction. This slice wires web material into existing enforcement; it does not implement every shell/MCP sandbox.

**Do not change:** No new web-local permission stack, model-issued grant, automatic trust from final pass or broader exemption for yolo-like flags.

**Proof required:** Real controlled action/egress sink stays untouched for each negative case and changes exactly once for a matching valid grant. Repeat across general security modes and ordinary allow-all. Provide payload/version and grant-scope evidence without secrets.

**Stop condition:** Missing effective-argument enforcement in the shared gate is an upstream blocker, not a reason to inspect only proposed model arguments.

## Acceptance Criteria

- A passed web answer remains untrusted for subsequent action and egress admission.
- Narrow grants bind actual action, data/version, final arguments, recipient, expiry and uses; mismatches and exhausted grants do not execute.
- Web restrictions survive off/observe/enforce, ordinary allow-all and permission-bypass flags; legitimate matching granted work still succeeds.

## Verification Plan

1. After a passed source, invoke controlled mutation and egress sinks with no grant, matching grant, changed arguments/data/recipient, expired grant and exhausted uses.
2. Repeat off/observe/enforce and normal allow-all; observe sink effects, not only permission return values.
3. Attach T31 matrix and scoped public-executor tests.

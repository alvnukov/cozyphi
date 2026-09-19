---
id: web-research-05-research-consent
title: 05 — Bind research consent to actual data, recipients and scope
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Only the explicit question and authorized sources enter model/search/site requests; no parent history or ambient files.
    - A research grant binds purpose, payload/data version, recipients or bounded classes, expiry and budget.
    - Unknown disclosure permission pauses with the actual recipient/payload; final dispatch revalidates redirects, retries and generated follow-ups.
verification_plan:
    - Use synthetic secrets and confidential-looking non-secret project text; capture model, search and site sinks separately.
    - Exercise deny, accept, headless pause, queued payload change, redirect, retry and generated follow-up.
    - Assert exact authorized bytes/recipients and zero denied dispatches; run scoped integration tests.
created_at: "2026-09-19T19:11:58.889841Z"
updated_at: "2026-09-19T19:11:58.889841Z"
---

## Body

**What to build:** One bounded research authorization instead of an approval for each font or an unrestricted web grant.

**Blocked by:** [03](web-research-03-model-binding.md), [model egress](security-model-egress.md), [dataflow grants](enforce-agent-dataflow-policy.md).

**Contract:** [Spec](../specs/protected-web-research.md), D8/D13, Q2/Q7/Q32/Q41, T15/T16. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Project the research question/sources into the shared grant and recipient machinery. Model provider, search service and target sites are distinct sinks. Show uncertain disclosure in a human-only decision with actual bounded payload; headless with no prior authority pauses. Recheck effective payload and recipient immediately before dispatch, including after queueing. Synthetic secrets test masking, but uncertain private context must still require consent after masking or LLM rewriting.

**Do not change:** No parallel permission engine, implicit parent-history transfer, per-resource nagging inside an unchanged valid grant, declassification by model, or general allow-all exception.

**Proof required:** Captured outbound payloads and consent events showing denied transfer emits nothing, authorized transfer emits only permitted data, and changed recipient/data forces a new decision.

**Stop condition:** Missing shared effective-request/grant APIs block integration; do not replace them with a web-local boolean.

## Acceptance Criteria

- Only the explicit question and authorized sources enter model/search/site requests; no parent history or ambient files.
- A research grant binds purpose, payload/data version, recipients or bounded classes, expiry and budget.
- Unknown disclosure permission pauses with the actual recipient/payload; final dispatch revalidates redirects, retries and generated follow-ups.

## Verification Plan

1. Use synthetic secrets and confidential-looking non-secret project text; capture model, search and site sinks separately.
2. Exercise deny, accept, headless pause, queued payload change, redirect, retry and generated follow-up.
3. Assert exact authorized bytes/recipients and zero denied dispatches; run scoped integration tests.

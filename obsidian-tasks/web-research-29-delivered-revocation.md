---
id: web-research-29-delivered-revocation
title: 29 — Pause dependent work and rebuild context after delivered evidence is revoked
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Previously delivered dependent results become unusable after revocation and affected actions pause before execution.
    - Continuation uses a clean/rebuilt permitted context or authorized successful resolution, never the same content plus warning.
    - Derivatives through fork/compaction/children are covered; completed external actions are not falsely described as undone.
verification_plan:
    - Exercise source-to-answer-to-compaction/fork/child derivations, then block from another session.
    - Inspect actual subsequent model payload and action sink, proving exclusion or safe refusal rather than just a UI badge.
    - Test an independent clean context control and record scoped lifecycle/action results.
created_at: "2026-09-19T19:17:19.178763Z"
updated_at: "2026-09-19T19:17:19.178763Z"
---

## Body

**What to build:** Stop acting on evidence already delivered before a later source block.

**Blocked by:** [25](web-research-25-post-web-grants.md), [28](web-research-28-inflight-revocation.md).

**Contract:** [Spec](../specs/protected-web-research.md), D7/D9/D12, Q14/Q21/Q34, T19/T20/T31. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Use shared derivation queries to mark affected prior results and pause dependent assignments. At next request/action boundary, exclude revoked material and derivatives using the existing clean-context/rebuild contract, or refuse continuation if exclusion cannot be proven. Notify the owning user safely. Include previously compacted/forked/delegated derivatives. Explicitly state that completed side effects cannot be rolled back by revocation.

**Do not change:** No in-place warning as sanitization, model-authored clean summary, deletion of incident evidence or new provenance graph.

**Proof required:** Deliver a synthetic source fact, derive a compacted/child result, block the source elsewhere, then attempt model/action continuation. Captured request lacks revoked derivatives or no request occurs; the controlled action sink remains untouched.

**Stop condition:** If shared context reconstruction cannot exclude a derivative, block continuation and record the missing shared capability.

## Acceptance Criteria

- Previously delivered dependent results become unusable after revocation and affected actions pause before execution.
- Continuation uses a clean/rebuilt permitted context or authorized successful resolution, never the same content plus warning.
- Derivatives through fork/compaction/children are covered; completed external actions are not falsely described as undone.

## Verification Plan

1. Exercise source-to-answer-to-compaction/fork/child derivations, then block from another session.
2. Inspect actual subsequent model payload and action sink, proving exclusion or safe refusal rather than just a UI badge.
3. Test an independent clean context control and record scoped lifecycle/action results.

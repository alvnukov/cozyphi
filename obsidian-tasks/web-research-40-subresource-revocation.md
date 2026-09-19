---
id: web-research-40-subresource-revocation
title: 40 — Revoke page derivatives when a contributing resource host is blocked
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Content-bearing third-party resource influence survives rendering, normalization, cache and answer derivation.
    - Blocking host B invalidates affected representations/results of page A without accusing A as the attack source.
    - Unknown influence restricts affected outputs; unrelated proven-independent resources/results remain usable.
verification_plan:
    - Create A-to-B script/image and independent C fixtures, then use real render/normalize/persistent-reuse/delivery operations.
    - Block B from another client and inspect release, subsequent payload and action sinks for affected A.
    - Check no fabricated A-host incident and retained dependencies; attach scoped revocation and reuse tests.
created_at: "2026-09-19T19:21:47.784422Z"
updated_at: "2026-09-19T19:34:10.155388Z"
---

## Body

**What to build:** Revoke a page-derived result when a blocked external script/image host influenced its consumed representation.

**Blocked by:** [28](web-research-28-inflight-revocation.md), [29](web-research-29-delivered-revocation.md), [35](web-research-35-verification-reuse.md), [38](web-research-38-rendered-pages.md), [39](web-research-39-visual-sources.md).

**Contract:** [Spec](../specs/protected-web-research.md), D4/D7/D9/D11, Q6/Q14/Q30/Q34, T19/T20/T23/T32. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Connect renderer/image dependencies to the delivered shared revocation propagation. Use 28 for in-flight results, 29 for already delivered derivatives and 35/its protected cache for persisted verification reuse. This task must not build those subsystems. Propagate through normalized snapshots, checked candidates and persisted reuse. Distinguish dependency invalidation from blame: affected A is not automatically a malicious-host block. Exercise ready, cached and previously delivered paths through their actual public operations.

**Do not change:** No citation-only reconstruction, ignored script influence, parent-domain expansion, private graph store or mock replacement for missing delivery/cache behavior.

**Proof required:** Controlled A uses B script/image plus independent C. After B block, A's affected ready/cached/delivered derivatives cannot release or drive action; independently proven C remains usable. Attribution remains B/unknown as evidence warrants.

**Stop condition:** Lost resource lineage restricts the broader page; absent predecessor behavior keeps this task blocked.

## Acceptance Criteria

- Content-bearing third-party resource influence survives rendering, normalization, cache and answer derivation.
- Blocking host B invalidates affected representations/results of page A without accusing A as the attack source.
- Unknown influence restricts affected outputs; unrelated proven-independent resources/results remain usable.

## Verification Plan

1. Create A-to-B script/image and independent C fixtures, then use real render/normalize/persistent-reuse/delivery operations.
2. Block B from another client and inspect release, subsequent payload and action sinks for affected A.
3. Check no fabricated A-host incident and retained dependencies; attach scoped revocation and reuse tests.

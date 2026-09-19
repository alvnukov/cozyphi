---
id: web-research-34-source-transfer
title: 34 — Isolate research access and preserve restrictions on explicit transfer
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Source/question/answer access is scoped to project/session even for identical public URLs and guessed IDs.
    - An explicit authorized transfer preserves provenance, restrictions and receiver permissions.
    - Global state exposes only necessary safe block/incident facts, not cross-project research history.
verification_plan:
    - Exercise owner, foreign project, foreign session, restored origin and guessed-ID access through public operations.
    - Grant and deny explicit transfers, then inspect recipient-visible source/provenance and actual model payload.
    - Assert global block facts contain no other-project research history; run scoped access/lifecycle tests.
created_at: "2026-09-19T19:18:45.29742Z"
updated_at: "2026-09-19T19:18:45.29742Z"
---

## Body

**What to build:** Scoped source access and explicit transfer between research contexts using the shared authority model.

**Blocked by:** [24](web-research-24-durable-lineage.md), [33](web-research-33-protected-cache.md), [dataflow grants](enforce-agent-dataflow-policy.md).

**Contract:** [Spec](../specs/protected-web-research.md), D8/D11, Q2/Q35/Q40/Q41, T15/T25. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Enforce owner/session/project checks at source resolution, answer retrieval and storage restore, not only listing. Introduce an explicit bounded user-authorized transfer through existing grant machinery; bind recipient and source version. Preserve restrictions and source dependencies. Same-URL content deduplication, if already supported, must not bypass access checks or reveal another project's query metadata.

**Do not change:** No automatic global source library, ID-as-capability shortcut, cleared provenance or content-bearing global incident feed.

**Proof required:** Two projects/sessions requesting the same URL; guessed references and copied IDs deny without leak. Authorized transfer succeeds with identical snapshot/provenance, while changed recipient/version requires fresh permission.

**Stop condition:** Missing owner metadata defaults restricted; do not infer ownership from current tab or filesystem location.

## Acceptance Criteria

- Source/question/answer access is scoped to project/session even for identical public URLs and guessed IDs.
- An explicit authorized transfer preserves provenance, restrictions and receiver permissions.
- Global state exposes only necessary safe block/incident facts, not cross-project research history.

## Verification Plan

1. Exercise owner, foreign project, foreign session, restored origin and guessed-ID access through public operations.
2. Grant and deny explicit transfers, then inspect recipient-visible source/provenance and actual model payload.
3. Assert global block facts contain no other-project research history; run scoped access/lifecycle tests.

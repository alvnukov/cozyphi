---
id: web-research-13-source-followup
title: 13 — Reuse immutable sources for an authorized follow-up question
status: blocked
priority: medium
model_level: low
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Follow-up defaults to the original immutable snapshot and runs fresh question-specific extraction/final screening.
    - Unknown, expired or unauthorized references give explicit safe outcomes rather than fetching current content.
    - Source metadata remains host-owned and reference possession does not grant access.
verification_plan:
    - Ask two questions of one source, mutate the remote fixture between them, and inspect fetch/model counts.
    - Exercise expired, guessed and foreign references and assert no implicit network request or foreign content.
    - Record stable source hash/version plus fresh checked response identities and scoped test results.
created_at: "2026-09-19T19:13:21.194701Z"
updated_at: "2026-09-19T19:13:21.194701Z"
---

## Body

**What to build:** Ask another question of a still-available source in the same authorized session without silently refetching it.

**Blocked by:** [11](web-research-11-static-answer.md).

**Contract:** [Spec](../specs/protected-web-research.md), D3/D11, Q18/Q19/Q20/Q40, T07/T23/T25. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Resolve existing source references through the tracer's ownership/identity boundary. Reuse bytes and provenance, not prior answer or authority. In this task source checks may run again; successful-check caching is deferred to 35. Return explicit missing/expired/inaccessible status without leaking another session's research. Retain bounded temporary snapshots only; persistence is not required.

**Do not change:** No implicit refresh, cross-project sharing, persistent cache, or new ID/crypto scheme.

**Proof required:** Fetch counter stays unchanged on a valid follow-up while new extraction/final calls occur; changing the remote fixture cannot alter the original snapshot. Invalid IDs trigger neither fetch nor content leak.

**Stop condition:** Missing scope metadata must deny, not fall back to global URL lookup.

## Acceptance Criteria

- Follow-up defaults to the original immutable snapshot and runs fresh question-specific extraction/final screening.
- Unknown, expired or unauthorized references give explicit safe outcomes rather than fetching current content.
- Source metadata remains host-owned and reference possession does not grant access.

## Verification Plan

1. Ask two questions of one source, mutate the remote fixture between them, and inspect fetch/model counts.
2. Exercise expired, guessed and foreign references and assert no implicit network request or foreign content.
3. Record stable source hash/version plus fresh checked response identities and scoped test results.

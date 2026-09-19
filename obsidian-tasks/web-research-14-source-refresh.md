---
id: web-research-14-source-refresh
title: 14 — Refresh a source explicitly without rewriting old evidence
status: blocked
priority: medium
model_level: low
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Explicit refresh creates a distinct immutable snapshot and performs new required checks.
    - Earlier source references still resolve only their original content or explicit expiry state.
    - Failed refresh cannot replace a prior snapshot or release unchecked new bytes.
verification_plan:
    - Use changing/unchanged source fixtures and explicit refresh through public web operations.
    - Verify old references, new retrieval metadata, permission rejection and failure rollback.
    - Attach source/version comparison and exact scoped test commands.
created_at: "2026-09-19T19:13:21.279922Z"
updated_at: "2026-09-19T19:13:21.279922Z"
---

## Body

**What to build:** A deliberate refresh operation that acquires a new source version without rewriting historical evidence.

**Blocked by:** [13](web-research-13-source-followup.md).

**Contract:** [Spec](../specs/protected-web-research.md), D3/D11, Q18/Q19, T07/T23. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Route explicit refresh through current permission, acquisition and snapshot checks. Retain old identity/content and return a new host-owned reference with real retrieval time. Revalidate site blocks and configuration rather than copying a previous pass. Add success, unchanged-body, changed-body, denied and failed-refresh cases according to existing immutable identity rules.

**Do not change:** No background refresh, mutable URL identity, silent retry, persistent storage or discarded provenance.

**Proof required:** Old/new references with actual content identity and timestamps; independent follow-ups reproduce the correct version. A rejected refresh produces no new releasable source and leaves old evidence unchanged subject to current revocation.

**Stop condition:** If identity rules for unchanged bytes are unspecified, resolve that narrow contract without promising a fresh unique content hash for identical data.

## Acceptance Criteria

- Explicit refresh creates a distinct immutable snapshot and performs new required checks.
- Earlier source references still resolve only their original content or explicit expiry state.
- Failed refresh cannot replace a prior snapshot or release unchecked new bytes.

## Verification Plan

1. Use changing/unchanged source fixtures and explicit refresh through public web operations.
2. Verify old references, new retrieval metadata, permission rejection and failure rollback.
3. Attach source/version comparison and exact scoped test commands.

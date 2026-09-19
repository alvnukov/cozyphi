---
id: web-research-09-exact-citations
title: 09 — Validate host-owned sources and exact normalized citations
status: blocked
priority: high
model_level: low
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Citations resolve only host-issued immutable source IDs and checked normalized ranges.
    - Wrong quote, source/version, URL, range or coverage prevents release.
    - URLs/timestamps and version are host-owned; exact quote matching is not labelled truth or safety proof.
verification_plan:
    - Submit controlled replies with one field corrupted at a time; assert rejection at the consumer boundary.
    - Verify exact byte/content correspondence under the documented coordinate system using Unicode and code/table fixtures.
    - Attach accepted/rejected fixture identifiers, public outcomes and changed-package test results.
created_at: "2026-09-19T19:11:59.235419Z"
updated_at: "2026-09-19T19:11:59.235419Z"
---

## Body

**What to build:** Reproducible citations for the one-source tracer using the minimal source contract already established in 06.

**Blocked by:** [06](web-research-06-single-source-tracer.md).

**Contract:** [Spec](../specs/protected-web-research.md), D3/D6/D11, Q18/Q29, T07. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Extract quoted text from immutable normalized material or compare it exactly. Validate range boundaries using the representation's declared units; include Unicode, empty/end ranges and changed normalization versions. Replace model-authored source URLs with host provenance, not invented repair. Refuse unverifiable citations and preserve candidate identity for final screening. Add small validation fixes and public-boundary fixtures only.

**Do not change:** No citation auto-correction by model, new source registry architecture, PDF parser or assertion that a matching quote is true.

**Proof required:** One accepted code/table/Unicode quote and a rejection table for forged ID, stale version, wrong URL, altered text, out-of-range and unchecked range. Retain exact fixture, normalized representation and emitted reference for reproduction.

**Stop condition:** If position units or normalization identity are undefined by the existing tracer contract, ask for contract resolution before writing conversions.

## Acceptance Criteria

- Citations resolve only host-issued immutable source IDs and checked normalized ranges.
- Wrong quote, source/version, URL, range or coverage prevents release.
- URLs/timestamps and version are host-owned; exact quote matching is not labelled truth or safety proof.

## Verification Plan

1. Submit controlled replies with one field corrupted at a time; assert rejection at the consumer boundary.
2. Verify exact byte/content correspondence under the documented coordinate system using Unicode and code/table fixtures.
3. Attach accepted/rejected fixture identifiers, public outcomes and changed-package test results.

---
id: web-research-17-independent-partials
title: 17 — Combine independent evidence and report safe partial answers
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Per-source extraction is checked before combination; a rejected source cannot influence a partial result.
    - Unknown dependency independence withholds the affected synthesis rather than deleting only its citation.
    - Partial answers identify missing coverage and disagreements; primary-source questions do not mechanically require two sources.
verification_plan:
    - Run one-primary-source, conflicting-source, unavailable-source and rejected-source fixtures.
    - Capture synthesis inputs as well as final output to prove rejected material did not merely lose its citation.
    - Assert completeness/conflict metadata and final screening; run scoped integration tests.
created_at: "2026-09-19T19:14:41.914926Z"
updated_at: "2026-09-19T19:14:41.914926Z"
---

## Body

**What to build:** Research with multiple sources and produce an honest checked partial answer when some fail.

**Blocked by:** [12](web-research-12-question-search.md), [16](web-research-16-consumed-coverage.md).

**Contract:** [Spec](../specs/protected-web-research.md), D5/D6/D14, Q4/Q12/Q28/Q31, T06/T10/T18/T29. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Keep source-specific extraction contexts separate and track dependencies into synthesis. Only admitted independent outputs enter a candidate; final-screen the exact combined answer. For failure or budget exhaustion, construct a partial answer only from uncontaminated evidence and explicit safe reasons. Distinguish exact official-source questions from contested comparisons; retain conflicts instead of manufacturing consensus.

**Do not change:** No citation-only removal of rejected influence, fixed two-source rule, truth guarantee or repeated checker sampling.

**Proof required:** Three controlled sources with distinct sentinel facts: one rejected, two conflicting. Rejected sentinels never appear in synthesis input/output; the released answer identifies the remaining conflict and partial coverage. An inseparable contaminated synthesis must be withheld.

**Stop condition:** Missing dependency precision restricts the whole affected result.

## Acceptance Criteria

- Per-source extraction is checked before combination; a rejected source cannot influence a partial result.
- Unknown dependency independence withholds the affected synthesis rather than deleting only its citation.
- Partial answers identify missing coverage and disagreements; primary-source questions do not mechanically require two sources.

## Verification Plan

1. Run one-primary-source, conflicting-source, unavailable-source and rejected-source fixtures.
2. Capture synthesis inputs as well as final output to prove rejected material did not merely lose its citation.
3. Assert completeness/conflict metadata and final screening; run scoped integration tests.

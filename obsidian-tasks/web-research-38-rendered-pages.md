---
id: web-research-38-rendered-pages
title: 38 — Read rendered public pages with constrained actions and subrequests
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - A rendered public page enters the same immutable coverage and release path as static material.
    - Every navigation/subresource passes actual network/grant/common-budget checks; no user profile, cookies or ambient credentials are inherited.
    - Only reading, scrolling and constrained expansion are possible; forms/arbitrary clicks/transactions are denied and absent isolation disables rendering.
verification_plan:
    - Render controlled dynamic documentation with scripts/styles/fonts/images and one permitted constrained expansion.
    - Attempt private resources, credentials, forms, arbitrary actions and resource/total-allowance exhaustion; inspect sinks and cleanup.
    - Record resource provenance, consumed representation and shared budget counters; run scoped browser integration tests.
created_at: "2026-09-19T19:20:23.914993Z"
updated_at: "2026-09-19T19:34:10.066392Z"
---

## Body

**What to build:** One JS-rendered documentation workflow through the reviewed browser adapter, not universal browser automation.

**Blocked by:** [10](web-research-10-public-acquisition.md), [16](web-research-16-consumed-coverage.md), [18](web-research-18-research-budgets.md), [36](web-research-36-format-feasibility.md).

**Unresolved prerequisite:** Reviewed renderer/read-action contract and actual platform isolation for browser descendants, files, environment and network. Record landed implementation IDs before reopening.

**Contract:** [Spec](../specs/protected-web-research.md), D4/D5/D15, Q3/Q5/Q10/Q11/Q30/Q39, T09/T11/T12/T13/T30. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Use a fresh isolated profile and route all scripts/styles/fonts/images/redirects through approved policy and the common research allowance from 18. Per-worker resource limits supplement, never replace/reset, that total accounting. Permit only specified bounded read actions; ambiguous expansion is denied, not trusted by label. Capture immutable rendered content, consumed regions and all content-bearing third-party dependencies. Honor cancellation and aggregate runtime eligibility from 41; until it passes, prove integration only through controlled tests.

**Do not change:** No login/cookie import, forms, arbitrary model click tool, private budget/security stack or erased DOM/source provenance.

**Proof required:** Controlled documentation plus private subresource, form, cookie, redirect, resource-bomb and exhausted-total-budget fixtures. Forbidden sinks stay empty; allowed resources render with recorded derivation and shared accounting.

**Stop condition:** Incomplete subrequest mediation, common allowance or platform isolation disables rendering.

## Acceptance Criteria

- A rendered public page enters the same immutable coverage and release path as static material.
- Every navigation/subresource passes actual network/grant/common-budget checks; no user profile, cookies or ambient credentials are inherited.
- Only reading, scrolling and constrained expansion are possible; forms/arbitrary clicks/transactions are denied and absent isolation disables rendering.

## Verification Plan

1. Render controlled dynamic documentation with scripts/styles/fonts/images and one permitted constrained expansion.
2. Attempt private resources, credentials, forms, arbitrary actions and resource/total-allowance exhaustion; inspect sinks and cleanup.
3. Record resource provenance, consumed representation and shared budget counters; run scoped browser integration tests.

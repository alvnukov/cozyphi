---
id: web-research-15-checked-excerpts
title: 15 — Return exact checked excerpts without a raw-content escape hatch
status: blocked
priority: medium
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Excerpt returns exact bounded normalized text from an authorized immutable range, not HTML/raw bytes.
    - The full emitted excerpt is covered by source checks and its own final screening.
    - Invalid, unchecked or revoked ranges fail closed; code and commands can be returned as cited data.
verification_plan:
    - Request valid/invalid code and Unicode ranges through the public operation and compare exact normalized contents.
    - Inject final-candidate rejection and revocation; assert no fallback text or checker prose reaches the parent.
    - Record source range/version, screened candidate identity and scoped tests.
created_at: "2026-09-19T19:14:41.744233Z"
updated_at: "2026-09-19T19:14:41.744233Z"
---

## Body

**What to build:** Request exact technical text without reintroducing raw access.

**Blocked by:** [09](web-research-09-exact-citations.md), [13](web-research-13-source-followup.md).

**Contract:** [Spec](../specs/protected-web-research.md), D3/D5/D6/D14, Q4/Q8/Q15/Q29, T06/T07/T10. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Resolve an authorized normalized range, extract it deterministically and feed the exact candidate through the existing release contract. State coordinate units, normalized representation/version and coverage. Preserve code formatting within safe host framing. Checker output is a verdict, not a rewritten excerpt. Refuse impossible ranges and truncated checking instead of silently shortening a supposedly exact quote.

**Do not change:** No raw override, model repair of exact text, unchecked HTML or command execution.

**Proof required:** Fixture-to-output equality for code, Unicode and attack-example documentation; final-check candidate equals emitted content. A decoy/injection in the candidate prevents release even when earlier source checks passed.

**Stop condition:** Unclear coverage fails closed; source validation and final screening cannot be skipped for an exact-text request.

## Acceptance Criteria

- Excerpt returns exact bounded normalized text from an authorized immutable range, not HTML/raw bytes.
- The full emitted excerpt is covered by source checks and its own final screening.
- Invalid, unchecked or revoked ranges fail closed; code and commands can be returned as cited data.

## Verification Plan

1. Request valid/invalid code and Unicode ranges through the public operation and compare exact normalized contents.
2. Inject final-candidate rejection and revocation; assert no fallback text or checker prose reaches the parent.
3. Record source range/version, screened candidate identity and scoped tests.

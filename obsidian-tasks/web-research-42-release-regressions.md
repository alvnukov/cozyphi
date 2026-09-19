---
id: web-research-42-release-regressions
title: 42 — Add fixed attack and benign regressions at the release boundary
status: blocked
priority: high
model_level: low
task_type: test
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - A bounded fixed regression corpus observes actual parent release, not just checker classifications.
    - Corpus covers forged citations, source/candidate injection, metadata/error/tool-call payloads, exact refresh/excerpts and rejected-source contamination.
    - Each case maps to requirement and existing public test seam with expected safe/benign outcome; no tests silently skipped as success.
verification_plan:
    - Run only the new/changed release fixture cases and their public harness package.
    - Record expected/actual sink sentinels and requirement IDs with positive controls and failing-before evidence.
    - Ensure source refresh regression uses the real 14 operation; classify skips/unavailable separately from pass.
created_at: "2026-09-19T19:21:47.956136Z"
updated_at: "2026-09-19T19:21:47.956136Z"
---

## Body

**What to build:** Extend the established public research fixtures with a fixed release-boundary attack regression set and reproducible evidence index.

**Blocked by:** [07](web-research-07-invalid-responses.md), [08](web-research-08-release-races.md), [09](web-research-09-exact-citations.md), [12](web-research-12-question-search.md), [14](web-research-14-source-refresh.md), [15](web-research-15-checked-excerpts.md), [16](web-research-16-consumed-coverage.md), [17](web-research-17-independent-partials.md), [26](web-research-26-incident-kinds.md).

**Contract:** [Spec](../specs/protected-web-research.md), Testing Decisions, T03–T08/T10/T14/T18/T23/T29. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Add data fixtures and assertions to existing public harness tests; do not invent a second eval framework. Include benign commands/security examples beside attacks. Check exact screened-versus-emitted candidate and all release surfaces. Record which cases use scripted models: they prove deterministic harness handling, not live detection quality. Reuse existing attack-corpus/reporting conventions from the shared security eval owner.

**Do not change:** No broad production fixes, live model calls, new fuzzing infrastructure or pass from all-refusal behavior.

**Proof required:** Versioned case manifest, expected/observed parent-release sentinels, positive controls and scoped regression output. Verify representative tests fail when their guard is temporarily bypassed, restoring the experiment.

**Stop condition:** Missing public seam or discovered runtime defect becomes a blocker/owned bug rather than an open-ended expansion of this low task.

## Acceptance Criteria

- A bounded fixed regression corpus observes actual parent release, not just checker classifications.
- Corpus covers forged citations, source/candidate injection, metadata/error/tool-call payloads, exact refresh/excerpts and rejected-source contamination.
- Each case maps to requirement and existing public test seam with expected safe/benign outcome; no tests silently skipped as success.

## Verification Plan

1. Run only the new/changed release fixture cases and their public harness package.
2. Record expected/actual sink sentinels and requirement IDs with positive controls and failing-before evidence.
3. Ensure source refresh regression uses the real 14 operation; classify skips/unavailable separately from pass.

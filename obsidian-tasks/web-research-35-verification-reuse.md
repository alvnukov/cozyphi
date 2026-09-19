---
id: web-research-35-verification-reuse
title: 35 — Reuse exact source-check evidence without caching authority
status: blocked
priority: medium
model_level: low
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Source-check reuse requires exact material/coverage, compatible checking context including relevant question and supplied context, and compatible model/prompt/policy/processing versions within TTL.
    - A new question still uses quarantined extraction and final screening; unknown checking-context compatibility forces a source-check miss.
    - Revocation, changed configuration, missing key or expired snapshot prevents reuse regardless of a previous pass.
verification_plan:
    - Repeat with a valid entry, then vary each material/coverage/version field, TTL and relevant checking context independently.
    - Hold snapshot/ranges/versions fixed while changing question/supplied context; assert unknown/incompatible context causes a miss and extraction/final remain fresh.
    - Block the hostname before use and inspect release refusal; attach scoped tests, stage counts and the full invalidation matrix.
created_at: "2026-09-19T19:20:23.659107Z"
updated_at: "2026-09-19T19:34:09.979163Z"
---

## Body

**What to build:** Reuse valid source-screening evidence without caching trust or skipping question-specific work.

**Blocked by:** [16](web-research-16-consumed-coverage.md), [27](web-research-27-host-blocks.md), [33](web-research-33-protected-cache.md).

**Contract:** [Spec](../specs/protected-web-research.md), D5/D6/D11/D13, Q19/Q20/Q29, T19/T23/T24. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Extend the existing artifact cache with the approved exact verification identity and bounded lifetime. Compare material/coverage/version fields AND the checking context actually supplied to safety: relevant question, permitted surrounding data and combined consumed inputs. Same bytes and prompt version alone do not establish compatibility. Unknown/missing compatibility is a miss; do not prescribe a new key scheme or trust model judgement to declassify context. Apply current ownership/revocation at every use. Count cache-hit work honestly and do not claim detection of hidden provider drift. Test values are not measured defaults.

**Do not change:** No cached final-answer authority, trust bit, skip of fresh extraction/final screening or TTL-based unblock.

**Proof required:** Field-by-field invalidation and call captures: valid hit reduces only source-check work. Keep snapshot/ranges/model/policy versions identical but change relevant question/supplied context; incompatible or unknown compatibility must rerun safety. Site block defeats an unexpired hit.

**Stop condition:** Undefined identity/context-compatibility rules invalidate reuse until resolved.

## Acceptance Criteria

- Source-check reuse requires exact material/coverage, compatible checking context including relevant question and supplied context, and compatible model/prompt/policy/processing versions within TTL.
- A new question still uses quarantined extraction and final screening; unknown checking-context compatibility forces a source-check miss.
- Revocation, changed configuration, missing key or expired snapshot prevents reuse regardless of a previous pass.

## Verification Plan

1. Repeat with a valid entry, then vary each material/coverage/version field, TTL and relevant checking context independently.
2. Hold snapshot/ranges/versions fixed while changing question/supplied context; assert unknown/incompatible context causes a miss and extraction/final remain fresh.
3. Block the hostname before use and inspect release refusal; attach scoped tests, stage counts and the full invalidation matrix.

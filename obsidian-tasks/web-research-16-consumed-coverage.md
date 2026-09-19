---
id: web-research-16-consumed-coverage
title: 16 — Check exactly the document regions and combinations actually consumed
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Every region consumed by extraction/synthesis is covered, including supplied surrounding context.
    - Checking one fragment never marks the full document passed; expanding consumed material requires new checks.
    - Chunking accounts for combined context; unchecked concatenations and visual regions cannot inherit isolated fragment passes.
verification_plan:
    - Run narrow-selection, expansion, overlap, cross-chunk and truncation fixtures through public research/excerpt operations.
    - Compare provider payload regions with coverage/version records; assert no whole-document clearance.
    - Use deterministic fixtures and scoped tests; include failing combined-context case and exact released ranges.
created_at: "2026-09-19T19:14:41.830149Z"
updated_at: "2026-09-19T19:14:41.830149Z"
---

## Body

**What to build:** Answer a focused question over part of a large normalized document without checking or certifying unused content.

**Blocked by:** [15](web-research-15-checked-excerpts.md).

**Contract:** [Spec](../specs/protected-web-research.md), D5/D6/D11, Q19/Q29, T08/T23. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Use deterministic local selection to choose bounded candidate regions without sending the whole document to a model. Record consumed ranges/context against existing immutable identity. Admit expanded and combined inputs before extraction; do not combine independently checked fragments in a new unchecked context. Exercise the policy first on a synthetic paginated text fixture; PDF/visual adapters later map their own regions to this contract.

**Do not change:** No new parser, whole-document pass bit, model access to hidden remainder or heuristic guarantee against cross-chunk injection.

**Proof required:** Captured model inputs and coverage records match exactly for narrow, overlapping and expanded requests. Place an attack split across chunks and prove the combined consumed candidate cannot bypass admission. Unused pages must not appear in model requests.

**Stop condition:** If provider truncation makes actual consumption unknown, stop the affected answer rather than infer full coverage.

## Acceptance Criteria

- Every region consumed by extraction/synthesis is covered, including supplied surrounding context.
- Checking one fragment never marks the full document passed; expanding consumed material requires new checks.
- Chunking accounts for combined context; unchecked concatenations and visual regions cannot inherit isolated fragment passes.

## Verification Plan

1. Run narrow-selection, expansion, overlap, cross-chunk and truncation fixtures through public research/excerpt operations.
2. Compare provider payload regions with coverage/version records; assert no whole-document clearance.
3. Use deterministic fixtures and scoped tests; include failing combined-context case and exact released ranges.

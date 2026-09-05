---
id: validate-edit-reliability-metrics-by-model
title: Correct edit reliability metrics and evaluate weak and strong models separately
status: todo
priority: high
model_level: high
task_type: test
parent_id: reliable-model-file-edits
tags:
    - review
    - eval
    - telemetry
    - reliability
acceptance_criteria:
    - Corrected retries are separated from unchanged retries; only successful trusted observations reset applicable recovery state, including editable grep/post-write outcomes.
    - Legacy and new codes map to stable semantic categories, with historical figures preserved and explicitly relabeled where original metric definitions were wrong.
    - Track per-model/version/effort/harness revision and task scenario, attempt denominators, final task correctness, retries/reads, token cost and latency.
    - Use the same scenario set before/after across at least one weaker and one stronger configured model, with repeated runs and sample counts; no unmeasured quality claim.
    - Measure write/shell fallbacks and final diff correctness so fewer edit errors cannot mask abandoned tasks or destructive rewrites.
    - Include multi-shift batches, duplicate anchors, long files, multiple files, compaction/resume, external modifications and refusing/repeating fake-provider trajectories; distinguish deterministic tests from real-model evals.
    - Document dependency on host-side loop recovery for any broad claim that weak-model retry loops are solved.
verification_plan:
    - Add a regression where invalid_ref is followed by corrected successful edit without read and assert it is not blind.
    - Test equivalent legacy/new refusal cohorts and actual chronological edit→write recovery.
    - Run controlled model evals only with explicit model settings and actual recorded results; report unmet targets honestly.
    - For Python changes run ruff check, mypy --strict, and focused analyzer tests.
created_at: "2026-09-05T06:42:03.972418Z"
updated_at: "2026-09-05T06:42:03.972418Z"
---

## Body

Audit on main 0032d62: scripts/analyze_edit_errors.py:214 marks any edit after a failed edit without read(mode=edit) as blind, even when arguments are corrected and the retry succeeds; reproduced using the analyzer itself (helper command 0a8fe2d12bb6ce1e2c7df6dae5e41d08). This contradicts the legal failure→corrected-retry path and the spec's 'unchanged re-call' definition. New stable classes snapshot_consumed/snapshot_evicted/anchor_not_observed etc. no longer fall under historical no_capability; comparing only old class names can manufacture improvement. Call has no model field; successful exact/rebased/recovered outcomes are not parsed separately. The edit→write count intersects paths across a whole session without checking event order, and read failure also resets the blind flag. The epic's raw counts -70%/-80% need denominators and matched cohorts. Do not use scripted good calls as evidence of model behavior. Existing claude-stuck-detector is the separate, still-todo dependency for host-side replay recovery; avoid duplicating it.

## Acceptance Criteria

- Corrected retries are separated from unchanged retries; only successful trusted observations reset applicable recovery state, including editable grep/post-write outcomes.
- Legacy and new codes map to stable semantic categories, with historical figures preserved and explicitly relabeled where original metric definitions were wrong.
- Track per-model/version/effort/harness revision and task scenario, attempt denominators, final task correctness, retries/reads, token cost and latency.
- Use the same scenario set before/after across at least one weaker and one stronger configured model, with repeated runs and sample counts; no unmeasured quality claim.
- Measure write/shell fallbacks and final diff correctness so fewer edit errors cannot mask abandoned tasks or destructive rewrites.
- Include multi-shift batches, duplicate anchors, long files, multiple files, compaction/resume, external modifications and refusing/repeating fake-provider trajectories; distinguish deterministic tests from real-model evals.
- Document dependency on host-side loop recovery for any broad claim that weak-model retry loops are solved.

## Verification Plan

1. Add a regression where invalid_ref is followed by corrected successful edit without read and assert it is not blind.
2. Test equivalent legacy/new refusal cohorts and actual chronological edit→write recovery.
3. Run controlled model evals only with explicit model settings and actual recorded results; report unmet targets honestly.
4. For Python changes run ruff check, mypy --strict, and focused analyzer tests.

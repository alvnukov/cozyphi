---
id: fix-plan-patch-api-batch-and-errors
title: 'plan tool: батч remove_criterion+add_criterion падает, ошибки патча не называют поле'
status: done
task_type: bug
tags:
    - plan
    - patch-api
    - harness
created_at: "2026-08-29T17:39:59.327039Z"
updated_at: "2026-09-02T19:01:40.130827Z"
---

## Body

Found while testing the plan tool in the harness (2026-08-29).

1. Batch patch bug: a single patch with remove_criterion + add_criterion fails with "op N: plan success criterion is required"; the same ops applied as separate patches succeed. Batch ops appear to be validated against the pre-patch snapshot, not sequentially.
2. insert_step without before/after fails without naming the missing anchor — the error should say an anchor is required.
3. A malformed patch (unknown/empty fields) returns "transition fields need one of the lifecycle actions (start, complete, block, resume, cancel, reopen)" instead of naming the offending field — misleading for an action=patch request.

Repro: all observed through the harness plan tool against the running session; see internal/session/plan_patch.go for the patch application path.

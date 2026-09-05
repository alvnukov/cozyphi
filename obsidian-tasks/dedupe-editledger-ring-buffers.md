---
id: dedupe-editledger-ring-buffers
title: Share the bounded ordered set between live and dead ledger snapshots
status: done
priority: low
model_level: medium
task_type: refactor
parent_id: reliable-model-file-edits
tags:
    - review
    - refactor
acceptance_criteria:
    - Live and dead snapshot bookkeeping use one shared bounded ordered-set implementation.
    - Line spans are a named type in editledger used by Resolution, ApplyHashlineEdit and successorGrantFor.
    - Existing editledger and writetool tests pass unchanged in behaviour.
verification_plan:
    - go test ./internal/tools/editledger/... ./internal/tools/writetool/... ./internal/tools/...
    - go vet on the changed packages.
created_at: "2026-09-05T07:09:33.594609Z"
updated_at: "2026-09-05T10:14:27.464238Z"
---

## Body

**Found by:** two-axis code review of the epic on main 0032d62 (commits 8492dec, 545aae1, cfd9083). Judgement calls from the Fowler smell baseline, not standard breaches.

**Duplicated code:** `remember`/`revive`/`deadOrder` in internal/tools/editledger/ledger.go:352-380 reproduce the bounded insertion-ordered ring of `track`/`forget`/`order` (:324-349), including the same linear delete-from-order loop. One small ordered-set type with insert/remove/evict-oldest would serve both.

**Primitive obsession / data clump:** line ranges travel as `[][2]int` across `Resolution.Lines` (ledger.go:114), the fourth return of `ApplyHashlineEdit` (internal/tools/writetool/hashline.go:479), `successorGrantFor` (:567) and internal/tools/writetool/write.go:112. A `Span{From, To}` in editledger would name what `[2]int{from, from+dstLen-1}` means and shrink the exported surface `ApplyHashlineEdit` now shows.

**Scope:** internal restructuring only; bounds (maxTrackedSnapshots=16, maxRememberedDispositions) and behaviour unchanged.

**Accepted and integrated (2026-09-05).** Both bounded snapshot orders use orderedSet with separate limits. editledger.Span is shared by resolution, apply and successor calculation. Integrated 694b438; public behavior and relevant race tests pass. Final implementation integrated into main at 66d047d. Full make fmt-check passed, followed by scoped formatting of final lint corrections and merged upstream files; final make lint test passed (QUALITY_EXIT=0). Relevant race gate passed (RACE_EXIT=0). Python ruff and mypy --strict passed. Evidence: doc/edit-reliability-evaluation.md and local .mcp-ai-helper/notes/edit-eval-20260905/.

## Acceptance Criteria

- Live and dead snapshot bookkeeping use one shared bounded ordered-set implementation.
- Line spans are a named type in editledger used by Resolution, ApplyHashlineEdit and successorGrantFor.
- Existing editledger and writetool tests pass unchanged in behaviour.

## Verification Plan

1. go test ./internal/tools/editledger/... ./internal/tools/writetool/... ./internal/tools/...
2. go vet on the changed packages.

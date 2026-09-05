---
id: unify-edit-refusal-codes
title: Derive edit refusal codes and texts from one source
status: todo
priority: low
model_level: medium
task_type: refactor
parent_id: reliable-model-file-edits
tags:
    - review
    - refactor
acceptance_criteria:
    - No outcome code string literal remains in writetool; codes come from editledger.
    - Edits are parsed once per call on the authorized path.
    - Refusal wire format (stable code prefix, Do not retry line, one next step) is unchanged; existing tests stay green.
verification_plan:
    - go test ./internal/tools/... and grep for "\[edit:" literals outside EditRefusal.Error.
    - python3 scripts/analyze_edit_errors.py still classifies the stable codes.
created_at: "2026-09-05T07:09:25.389371Z"
updated_at: "2026-09-05T07:09:25.389371Z"
---

## Body

**Found by:** two-axis code review of the epic on main 0032d62 (commit 8492dec, task typed-edit-capability-outcomes). Judgement calls from the Fowler smell baseline, not standard breaches.

**Repeated switches / primitive obsession:** `Outcome.Code()` in internal/tools/editledger/ledger.go:53 and `refusalForOutcome` in internal/tools/writetool/hashline.go:268-330 both switch over `Outcome`; the latter re-types every code as a string literal ("snapshot_consumed", … "no_capability") instead of calling `outcome.Code()`. `EditRefusal.Code` is a bare string and "invalid_ref" is spelled six times in hashline.go. Only `TestOutcomeCodes` keeps the two lists aligned.

**Duplicated code:** `newHashlineMismatchError` (hashline.go:847) hand-writes the "[edit:tag_changed] … Do not retry the same call unchanged." format, re-implementing `EditRefusal.Error()` for one type. The "edit requires hash" refusal appears at hashline.go:205 and :375; the `edits[%d]: %s` parse refusal at :218 and :488, and on the authorized path edits are parsed twice (`toParsedEdit` at :214 and :484).

**Fix direction:** a typed code (e.g. `editledger.Code`) used by both packages, `refusalForOutcome` keyed on `outcome.Code()` with a map or table for What/Next, the mismatch error built through `EditRefusal`, edits parsed once and threaded through.

## Acceptance Criteria

- No outcome code string literal remains in writetool; codes come from editledger.
- Edits are parsed once per call on the authorized path.
- Refusal wire format (stable code prefix, Do not retry line, one next step) is unchanged; existing tests stay green.

## Verification Plan

1. go test ./internal/tools/... and grep for "\[edit:" literals outside EditRefusal.Error.
2. python3 scripts/analyze_edit_errors.py still classifies the stable codes.

---
id: sync-edit-capability-doc-with-code
title: Sync doc/edit-capability.md outcome names with the ledger
status: todo
priority: low
model_level: low
task_type: docs
parent_id: reliable-model-file-edits
tags:
    - review
    - docs
acceptance_criteria:
    - Every outcome and code named in doc/edit-capability.md exists in editledger or writetool with the same spelling.
    - The snapshot_consumed next step in the doc and in the refusal text agree.
verification_plan:
    - grep each backticked outcome/code token from the doc against internal/tools/editledger and internal/tools/writetool.
created_at: "2026-09-05T07:09:14.886116Z"
updated_at: "2026-09-05T07:09:14.886116Z"
---

## Body

**Found by:** two-axis code review of the epic on main 0032d62 (commits 545aae1, cfd9083, 8492dec).

**Drift:**
- doc/edit-capability.md:28 says a uniform-shift re-anchor "grants with `Outcome=Rebased`". No such outcome exists: internal/tools/editledger/ledger.go returns `Granted` with a non-zero `Delta`. The design contract names a constant the code does not have (AGENTS.md Readability: names say the thing).
- doc/edit-capability.md:66 gives snapshot_consumed the next step "use the successor anchors from the last successful edit, or re-read"; the refusal in internal/tools/writetool/hashline.go:278 only says re-read. Decide which side is right; if the hint should mention successor anchors, that wording belongs to align-model-edit-capability-contract.
- typed-edit-capability-outcomes acceptance criteria name the code `no_snapshot`; code and doc use `no_capability`. Keep `no_capability`, note the alias nowhere else exists.

**Scope:** documentation only; no behaviour change.

## Acceptance Criteria

- Every outcome and code named in doc/edit-capability.md exists in editledger or writetool with the same spelling.
- The snapshot_consumed next step in the doc and in the refusal text agree.

## Verification Plan

1. grep each backticked outcome/code token from the doc against internal/tools/editledger and internal/tools/writetool.

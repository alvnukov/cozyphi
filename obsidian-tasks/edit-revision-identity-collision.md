---
id: edit-revision-identity-collision
title: Protect edit revision identity against short TAG collisions
status: done
priority: high
model_level: high
task_type: bug
parent_id: reliable-model-file-edits
tags:
    - review
    - reliability
    - data-loss
acceptance_criteria:
    - A different observed revision cannot authorize an edit solely because its four-character display TAG collides.
    - A public EditTool regression covering a colliding TAG and unchanged range endpoints refuses and preserves the external content.
    - Authoritative revision identity is checked both when resolving a grant and when committing a write; normalization rules and any accepted equivalences are explicit.
    - Existing successor, post-write, exact/rebased and external-change refusal behavior remains covered.
verification_plan:
    - Reproduce the collision case through ReadTool/EditTool or EditTool with an explicitly issued grant, using temporary files.
    - Run focused editledger/writetool tests; inspect related read/grep observation paths.
    - Run required format/lint/tests after implementation.
created_at: "2026-09-05T06:42:03.967366Z"
updated_at: "2026-09-05T07:42:44.102345Z"
---

## Body

Confirmed by the epic audit on main 0032d62 (2026-09-05). internal/util/hash.go:98 uses only the low 16 bits of FNV64a; editledger keys observations by that display TAG, and runParsedEdit/unchangedTagGuard compare the same truncated value. A deterministic temporary overlay test generated two distinct three-line contents start\nexternal_value_N\nend with the same TAG=C9CD. After authorizing the first revision, replacing the file externally with the second, then replacing range 1..3 using unchanged endpoint anchors, EditTool.Run succeeded and erased the external interior change. This is an inherited gap in the epic's exact-revision/fail-closed premise, not evidence that the epic introduced it. Evidence: helper command d767aab909165d38b143cbc5ab6644c7, TestEpicAuditExternalTagCollision. Keep model-visible references compact if desired, but separate display identifiers from authoritative revision identity.

**Done (2026-09-05, commit e094108, merge e19d1a8):** `util.Revision` (full FNV-1a 64) is the authoritative identity; `Revision.Tag()` is the 4-hex display form and `ComputeFileHash` now derives from it. The ledger keys snapshots by `(path, Revision)`, keeps at most one live snapshot per `(path, TAG)` (`admit` retires the older colliding one as `snapshot_evicted`), `Claim` resolves the quoted TAG to that snapshot and exposes `Resolution.Revision`/`Claim.Revision()`, `Authorize`/`Commit` take a `Revision`. read, grep and write authorize revisions. `runParsedEdit` compares the claim revision against disk (`staleRevision` → `tag_changed`, with a same-TAG-different-content message) and `unchangedRevisionGuard` repeats the full comparison before the swap (`changed_during_edit`). Regressions: `TestEditRefusesCollidingTagWithChangedContent` (public EditTool, colliding pair, external content survives), `TestVerifyGuardRefusesSameTagSwap`, `TestDifferentRevisionsCanShareOneTag`, ledger tests for collision retirement. Accepted equivalences documented on `RevisionOf` (LF normalization, trailing-whitespace trim). doc/edit-capability.md Principle 2 and the module table updated (also fixed the stale `Outcome=Rebased` wording); CHANGELOG Fixed line. Gates: build, race tests on util/editledger/readtool/greptool/writetool/tools, gofmt, one golangci-lint (0 issues). Merge conflicts with the earlier atomicfile change resolved by keeping both tests and both doc paragraphs.

## Acceptance Criteria

- A different observed revision cannot authorize an edit solely because its four-character display TAG collides.
- A public EditTool regression covering a colliding TAG and unchanged range endpoints refuses and preserves the external content.
- Authoritative revision identity is checked both when resolving a grant and when committing a write; normalization rules and any accepted equivalences are explicit.
- Existing successor, post-write, exact/rebased and external-change refusal behavior remains covered.

## Verification Plan

1. Reproduce the collision case through ReadTool/EditTool or EditTool with an explicitly issued grant, using temporary files.
2. Run focused editledger/writetool tests; inspect related read/grep observation paths.
3. Run required format/lint/tests after implementation.

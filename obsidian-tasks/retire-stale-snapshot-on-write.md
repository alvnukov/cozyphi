---
id: retire-stale-snapshot-on-write
title: Retire the pre-write snapshot when write mints its capability
status: done
priority: medium
model_level: medium
task_type: bug
parent_id: reliable-model-file-edits
tags:
    - review
    - capability
    - reliability
acceptance_criteria:
    - After a successful write, an edit with the pre-write TAG is refused by the ledger with a typed outcome, not by the on-disk TAG check.
    - A ledger test shows the pre-write snapshot is gone and remembered with a disposition after write.
    - tools_test.go and guard_test.go comments describe the tests they sit above.
verification_plan:
    - go test ./internal/tools/... with a new read→write→edit(old TAG) case asserting the outcome code.
    - Re-run TestEpicAudit* cases from edit-revision-identity-collision if merged first.
created_at: "2026-09-05T07:09:06.684687Z"
updated_at: "2026-09-05T07:55:12.062695Z"
---

## Body

**Found by:** two-axis code review of the epic on main 0032d62 (commit 5ccace6, task authorize-post-write-edits).

**What:** internal/tools/writetool/write.go:114 registers the post-write grant with plain `ledger.Authorize`, so the snapshot of the revision that existed before the write stays live in the ledger. A hashline `Commit` swaps the old TAG for the successor; `write` does not. An edit that still carries the pre-write TAG is granted by the ledger and is only refused by the on-disk TAG check in hashline.go (`[edit:tag_changed]`). Verified with a throwaway read→write→edit test: the file stays untouched, so the outcome is fail-closed, but the decision comes from the disk check, not from the capability module, which contradicts doc/edit-capability.md principle 3 (the ledger decides). The comment in internal/tools/tools_test.go:265-266 claims "the old revision's grant dies with the old TAG", which is not what the ledger does.

**Interaction:** while edit-revision-identity-collision is open, a stale pre-write snapshot whose 16-bit TAG collides with the new content would pass the disk check with anchors from the old revision. Retiring the snapshot on write removes that path independently of the collision fix.

**Also in the same commit:** internal/tools/writetool/guard_test.go:62-64 keeps the comment "A destination that is still where the gate left it writes normally…" above `TestFailedWriteMintsNoPostWriteGrant`; it describes `TestRunWriteUnderGuardStillWritesInsideWorkspace` (now comment-less at line 86). Move it back.

**Fix direction:** give the ledger a write-side commit (retire every snapshot of the path, remember them as consumed/superseded, authorize the new revision) and call it from write; fix both test comments.

**Done (2026-09-05, commit 949d7b2, merged 09d652a):** new `Ledger.Supersede(path, rev, anchors)` retires every live snapshot of the path (remembered as the new `SnapshotSuperseded` outcome, code `snapshot_superseded`) and authorizes the written revision; `write` calls it unconditionally after the swap; `refusalForOutcome` maps the outcome to `[edit:snapshot_superseded]` pointing at the write result's anchors. Ledger tests `TestLedgerSupersedeRetiresEverySnapshotOfPath` and `TestLedgerSupersedeSameRevisionStaysLive`, the tools chain test gains a stale pre-write-TAG edit refused by the ledger with the file untouched, both test comments fixed, doc/edit-capability.md and CHANGELOG updated. Gates: build, `go test -race` on editledger/writetool/tools, gofmt, golangci-lint clean.

## Acceptance Criteria

- After a successful write, an edit with the pre-write TAG is refused by the ledger with a typed outcome, not by the on-disk TAG check.
- A ledger test shows the pre-write snapshot is gone and remembered with a disposition after write.
- tools_test.go and guard_test.go comments describe the tests they sit above.

## Verification Plan

1. go test ./internal/tools/... with a new read→write→edit(old TAG) case asserting the outcome code.
2. Re-run TestEpicAudit* cases from edit-revision-identity-collision if merged first.

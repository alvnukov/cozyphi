---
id: edit-post-verify-writer-loss
title: Resolve the edit verify-to-rename lost-update guarantee
status: done
priority: high
model_level: high
task_type: bug
parent_id: reliable-model-file-edits
tags:
    - review
    - reliability
    - concurrency
    - data-loss
acceptance_criteria:
    - A documented concurrency contract distinguishes cooperating writers from arbitrary external writers and does not claim unsupported atomic compare-and-swap semantics.
    - Cooperating CozyPhi mutations of the same target are serialized across the intended session/job scope.
    - A deterministic interleaving regression covers an external change after verification and establishes the intended conflict handling or explicit limitation.
    - Preserve symlink/permission checks, crash-safe replacement, and cancellation behavior.
verification_plan:
    - Reproduce with EditTool.Run and a mutation guard writing distinct content on its second invocation.
    - Run focused atomicfile/writetool concurrency tests and relevant race tests.
    - Review the achievable guarantee before choosing synchronization or conflict-preservation design.
created_at: "2026-09-05T06:42:03.969363Z"
updated_at: "2026-09-05T07:36:19.806297Z"
---

## Body

Follow-up to completed fix-hashline-edit-atomic-write. Confirmed on main 0032d62 by TestEpicAuditWriterAfterVerify, helper command d767aab909165d38b143cbc5ab6644c7. atomicfile.write calls Verify, performs path checks and the second Guard, then os.Rename. The test injects an external file write during that second MutationGuard invocation, after Verify has accepted the old bytes. EditTool.Run returns success and overwrites the external content. No threads or timing sleeps are needed. Atomic replacement prevents torn writes but is not compare-and-swap. Audit the serialization of cooperating sessions/jobs and explicitly define what can be guaranteed for arbitrary external writers; merely adding another pre-rename read repeats the check-then-act problem. This predates this epic but contradicts its unconditional zero-silent-overwrites wording.

**Done (2026-09-05, merge 3c12c19, fix commit 81d88bb):** atomicfile now takes a process-wide ref-counted per-path lock (internal/atomicfile/pathlock.go, lexical key via filepath.Abs+Clean) after staging and holds it across the leaf Lstat, directory re-verification, second Guard, Verify and os.Rename; Verify moved to the very end so only the rename separates the judged bytes from the swap. Contract documented in the package doc and in doc/edit-capability.md "Concurrency contract": cooperating writers (all callers in the process) never lose an update silently, the loser refuses with changed_during_edit; arbitrary external writers keep the two-syscall residual window, which is stated, not claimed away. Tests: TestVerifyCatchesWriterLandingAfterTheGuard (mutation-checked: fails with Verify in the old position), TestCooperatingWritersSerializePerPath, TestWritersOnDifferentPathsDoNotBlockEachOther, TestPathLockRegistryDropsIdleEntries, TestLockKeyNormalizesSpelling, TestEditRefusesWriterLandingAfterVerify (public EditTool API, failed before the fix). Gates: go build, go test and go test -race on atomicfile and writetool, gofmt, golangci-lint 0 issues; -race -count=5 on atomicfile stable. CHANGELOG Unreleased entry added. Implemented by an Opus subagent, reviewed against the diff by the supervising session.

## Acceptance Criteria

- A documented concurrency contract distinguishes cooperating writers from arbitrary external writers and does not claim unsupported atomic compare-and-swap semantics.
- Cooperating CozyPhi mutations of the same target are serialized across the intended session/job scope.
- A deterministic interleaving regression covers an external change after verification and establishes the intended conflict handling or explicit limitation.
- Preserve symlink/permission checks, crash-safe replacement, and cancellation behavior.

## Verification Plan

1. Reproduce with EditTool.Run and a mutation guard writing distinct content on its second invocation.
2. Run focused atomicfile/writetool concurrency tests and relevant race tests.
3. Review the achievable guarantee before choosing synchronization or conflict-preservation design.

---
id: edit-revision-identity-collision
title: Protect edit revision identity against short TAG collisions
status: todo
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
updated_at: "2026-09-05T06:42:03.967366Z"
---

## Body

Confirmed by the epic audit on main 0032d62 (2026-09-05). internal/util/hash.go:98 uses only the low 16 bits of FNV64a; editledger keys observations by that display TAG, and runParsedEdit/unchangedTagGuard compare the same truncated value. A deterministic temporary overlay test generated two distinct three-line contents start\nexternal_value_N\nend with the same TAG=C9CD. After authorizing the first revision, replacing the file externally with the second, then replacing range 1..3 using unchanged endpoint anchors, EditTool.Run succeeded and erased the external interior change. This is an inherited gap in the epic's exact-revision/fail-closed premise, not evidence that the epic introduced it. Evidence: helper command d767aab909165d38b143cbc5ab6644c7, TestEpicAuditExternalTagCollision. Keep model-visible references compact if desired, but separate display identifiers from authoritative revision identity.

## Acceptance Criteria

- A different observed revision cannot authorize an edit solely because its four-character display TAG collides.
- A public EditTool regression covering a colliding TAG and unchanged range endpoints refuses and preserves the external content.
- Authoritative revision identity is checked both when resolving a grant and when committing a write; normalization rules and any accepted equivalences are explicit.
- Existing successor, post-write, exact/rebased and external-change refusal behavior remains covered.

## Verification Plan

1. Reproduce the collision case through ReadTool/EditTool or EditTool with an explicitly issued grant, using temporary files.
2. Run focused editledger/writetool tests; inspect related read/grep observation paths.
3. Run required format/lint/tests after implementation.

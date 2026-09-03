---
id: fix-compaction-fileops-carryforward
title: 'Compaction file-op carry-forward is dead: mismatched details types, never-set gate'
status: done
priority: medium
task_type: bug
parent_id: cozyphi-enterprise-code-review
tags:
    - reliability
    - compaction
    - review-2026-08
    - sector:agent-context
created_at: "2026-08-27T16:09:20.830482Z"
updated_at: "2026-08-27T23:27:35.544905Z"
---

## Body

internal/session/compaction/fileops.go:68 asserts comp.Compaction.Details.(session.CompactionDetails) but compact.go:271 stores compaction.CompactionDetails - a duplicate type at compact.go:306 (session.CompactionDetails lives at entry.go:165). The branch is also gated on FromExtension != nil (fileops.go:67) which nothing sets. Read/modified file lists never accumulate past the first compaction. Fix: keep one type, assert the persisted one, delete or set FromExtension.

---
id: fix-memory-catalog-atomic-write
title: MEMORY.md written non-atomically; three atomic-write shapes duplicated in repo
status: done
priority: medium
task_type: bug
parent_id: cozyphi-enterprise-code-review
tags:
    - reliability
    - memory
    - review-2026-08
    - sector:atomic-writes
created_at: "2026-08-27T16:09:20.842708Z"
updated_at: "2026-08-27T23:42:46.396823Z"
---

## Body

internal/memory/memory.go:223 uses plain os.WriteFile (compare-then-write, truncate in place) while the repo standard is temp+rename+sync (project.SaveUIState, harnesssettings.writeAtomicOwnerOnly - three atomic-write shapes, Duplicated Code). A crash or a concurrent Claude Code write leaves a torn catalog both agents then read. Fix: one shared atomic-write helper used by all three.

---
id: fix-hashline-edit-atomic-write
title: 'edit: non-atomic write and check-to-write window clobbers concurrent changes'
status: done
priority: medium
task_type: bug
parent_id: cozyphi-enterprise-code-review
tags:
    - reliability
    - hashline
    - review-2026-08
    - sector:atomic-writes
created_at: "2026-08-27T16:09:20.793471Z"
updated_at: "2026-08-27T23:42:46.395819Z"
---

## Body

internal/tools/writetool/hashline.go:164-193: os.ReadFile, anchor verification, then plain os.WriteFile. A concurrent writer (sub-agent, watch command, user editor) mutating the file inside that window is silently clobbered; a crash mid-write truncates. Stale anchors fail closed only up to the check. Fix: write temp file in the same dir, re-read and re-verify the TAG, then rename over the target.

---
id: fix-jobprogress-map-leak
title: Controller lastJobProgress sync.Map never evicted
status: done
priority: medium
task_type: bug
parent_id: cozyphi-enterprise-code-review
tags:
    - reliability
    - tui
    - review-2026-08
    - sector:background-lifetime
created_at: "2026-08-27T16:09:20.846306Z"
updated_at: "2026-08-28T06:42:07.697985Z"
---

## Body

controller.go:84, 275-286: keys (JobID+ToolUseID) are never evicted; a long session with many sub-agents grows it unboundedly. Evict on terminal job status.

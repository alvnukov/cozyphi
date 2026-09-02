---
id: fix-watch-session-bounds
title: 'Watches: flood bound is per-watch (8x limit) and finished watches are never pruned'
status: done
priority: medium
task_type: bug
parent_id: cozyphi-enterprise-code-review
tags:
    - reliability
    - watch
    - review-2026-08
    - sector:background-lifetime
created_at: "2026-08-27T16:09:20.83944Z"
updated_at: "2026-08-28T06:42:07.692459Z"
---

## Body

internal/watch/watch.go:244-254 meters FloodLimit per entry; with MaxLive=8 the session can see 160 events/min reach transcripts and reminders - AGENTS.md states 20 events/min as one of four session bounds. Fix: manager-level window or document the bound as per-watch. Also watch.go:207 is the only write to m.entries and there is no delete: each finished watch retains up to logLimit(200) x EventTextLimit(2000) ~= 400KB forever. Cap retained finished entries or drop logs at finish.

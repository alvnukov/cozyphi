---
id: fix-session-atomic-flush
title: 'Session persistence: non-atomic flush; torn tail makes session unresumable'
status: done
priority: high
task_type: bug
parent_id: cozyphi-enterprise-code-review
tags:
    - reliability
    - session
    - review-2026-08
created_at: "2026-08-27T16:09:20.828663Z"
updated_at: "2026-08-27T16:24:29.263792Z"
---

## Body

flushAllEntries (internal/session/manager.go:399) opens with O_TRUNC and rewrites in place; a crash mid-rewrite destroys the whole prior transcript. OpenSession (load.go:229-231) hard-errors on any undecodable line, so one torn append line makes the session unresumable while readSessionMeta tolerates it (load.go:84-89). Fix: temp-file+rename flush; on load drop a torn final line instead of failing. Also: Append mutates leafID/entries/byIDs before flush without rollback on error (manager.go:353-370), unlike persistPlanLocked (plan.go:240-248); and replacePlanLocked (plan.go:83) takes the lock itself unlike every other *Locked helper - rename replacePlan.

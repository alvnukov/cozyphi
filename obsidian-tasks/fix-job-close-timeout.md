---
id: fix-job-close-timeout
title: job.Manager.Close ignores its ctx and can hang shutdown forever
status: done
priority: medium
task_type: bug
parent_id: cozyphi-enterprise-code-review
tags:
    - reliability
    - jobs
    - review-2026-08
created_at: "2026-08-27T16:09:20.787794Z"
updated_at: "2026-08-27T22:36:26.574886Z"
---

## Body

Close(_ context.Context) (internal/job/manager.go:452) waits <-lj.done unconditionally; the controller passes a 3s timeout (controller.go:1581-1584) that does nothing. One wedged sub-agent hangs app shutdown. Fix: honor ctx in Close (return ctx.Err() and abandon the wait) or let Controller.Close proceed after the budget.

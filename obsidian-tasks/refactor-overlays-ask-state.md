---
id: refactor-overlays-ask-state
title: 'Overlays: six copies of one close/begin routine'
status: done
priority: medium
task_type: refactor
parent_id: cozyphi-enterprise-code-review
tags:
    - duplication
    - tui
    - review-2026-08
    - sector:tui-shell
created_at: "2026-08-27T16:09:20.864Z"
updated_at: "2026-08-28T11:49:08.879202Z"
---

## Body

overlays.go:190-275 (dismissPermission/resolvePermission/dismissContinue/resolveContinue), question.go:99-131, plus four begin* preludes repeating resolve-others/clearConnect/hide-composer/ActivityAwaitingApproval. Extract askState[T any]{reply chan T} with shared begin/dismiss/resolve.

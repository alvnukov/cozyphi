---
id: fix-slash-dispatch-error-surface
title: DispatchSlash discards Run errors; command failure surfacing is inconsistent
status: done
priority: medium
task_type: bug
parent_id: cozyphi-enterprise-code-review
tags:
    - tui
    - commands
    - review-2026-08
    - sector:tui-shell
created_at: "2026-08-27T16:09:20.857014Z"
updated_at: "2026-08-28T11:49:08.875968Z"
---

## Body

internal/tui/commands/registry.go:225 drops the error (only usage recording is gated); /resume and /theme toast inside Run, /model relies on editor.SetModel toasting (editor.go:898) - some failures surface twice, others never. Standardize: Run returns error, dispatcher toasts once.

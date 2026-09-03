---
id: refactor-settings-pane-policy-algebra
title: Settings pane implements plan-policy assignment algebra in the widget
status: done
priority: medium
task_type: refactor
parent_id: cozyphi-enterprise-code-review
tags:
    - architecture
    - tui
    - settings
    - review-2026-08
    - sector:tui-shell
created_at: "2026-08-27T16:09:20.860609Z"
updated_at: "2026-08-28T11:49:08.877742Z"
---

## Body

internal/tui/settings/pane.go:538-579,459-478: the pane's own comment says validation/merge/persistence remain behind Store, yet togglePermission, toggleOutsidePlan, assignmentRank, removeToolAssignments and recordRename implement policy algebra in the widget - violates the UI split. Move draft mutations onto harnesssettings.Draft methods; pane stays dumb rows. BLOCKED: settings pane is the in-flight plan-runtime feature - file after it lands and re-verify.

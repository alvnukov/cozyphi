---
id: refactor-controller-split
title: controller.go (1810 lines) owns 10+ responsibilities; extract asks, session switch, sidebar prefs, replay
status: done
priority: high
task_type: refactor
parent_id: cozyphi-enterprise-code-review
tags:
    - architecture
    - tui
    - review-2026-08
created_at: "2026-08-27T16:09:20.844487Z"
updated_at: "2026-08-27T22:12:01.754083Z"
---

## Body

One struct owns engine assembly/lifecycle, stream+prompt queue, permission/continue/question asks, plan-approval resume state machine, watch wake/streak scheduling, provider catalog+OAuth, hooks lifecycle, MCP/LSP status, sidebar UI-state persistence, replay projection, compaction. Extract at least: (a) generic ask[T] - askPermission/askContinue/askQuestion (controller.go:949-1038) repeat make-chan/publish/timeout/dismiss three times; (b) switchSession(reason, opts) - Resume and Clear share the identical sequence (controller.go:1152-1256); (c) a uistate store for SidebarPreferences/SaveSidebarWidth/SaveSidebarVisibility/SaveStopLimit load-mutate-save twins (controller.go:836-896); (d) ReplaySnapshot (controller.go:1260-1324) is session->widget projection, belongs in internal/tui/transcript per doc/tui.md. Editor follow-ups in the same pass: per-frame ctrl.LSPStatuses()/MCPStatuses()/sidebar.SetRuntime pushes with no change detection (editor.go:523-535, dup of initial push editor.go:141-147); repeated e.settings != nil && e.settings.Visible() x5 (editor.go:438-652) - extract modalActive(). NOTE: controller.go/editor.go are part of the in-flight plan-runtime feature - re-verify line numbers after it lands.

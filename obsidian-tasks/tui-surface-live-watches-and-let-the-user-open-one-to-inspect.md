---
id: tui-surface-live-watches-and-let-the-user-open-one-to-inspect
title: 'TUI: surface live watches and let the user open one to inspect'
status: done
priority: medium
task_type: issue
tags:
    - tui
    - watch
    - ux
created_at: "2026-09-01T21:28:45.530959Z"
updated_at: "2026-09-01T22:11:50.124788Z"
---

## Body

While cutting the v0.19.0 release (watch running `make fmt-check lint test`), nothing in the TUI showed that a watch existed at all: silence reads identically to "stopped" and to "still running", and there is no way to open a watch and look at its output/log on demand.

Proposed UX:
- A persistent, low-noise indicator of live watches (footer or sidebar): count/names, e.g. `⏱ 1 watch: release gates`.
- A keybinding that opens a watch list overlay: each row = watch label + state (running/armed), Enter or a key opens that watch's captured log (the same data `watch action=log` returns), Esc returns.
- Events keep arriving as transcript rows/reminders exactly as now (doc/watch.md invariants unchanged: 20 events/min, 8 watches, reminders never user messages).

Where: rendering in internal/components, wiring in internal/tui (widgets dumb, draw-on-demand); the manager side lives where the watch manager already is. Sub-agents/headless keep no manager, so the indicator simply hides when there is no manager.

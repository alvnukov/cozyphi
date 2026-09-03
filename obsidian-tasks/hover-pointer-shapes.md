---
id: hover-pointer-shapes
title: Hover pointer shapes via OSC 22
status: done
priority: medium
task_type: feature
tags:
    - phase1
    - ux
created_at: "2026-08-24T17:56:41.785424Z"
updated_at: "2026-08-24T18:06:39.121153Z"
---

## Body

Terminals (kitty 0.31+, ghostty, foot) support OSC 22 to change the mouse
pointer shape; xui already enables 1003 all-motion tracking and exposes
WriteRaw. Add a hover layer:

- components.PointerShaper interface + ShapePointer/ShapeText/ShapeResizeEW
  constants (CSS cursor names per the kitty spec).
- app: on every MouseEvent hit-test lastSurf, ask the deepest widget for its
  local shape, dedupe, emit OSC 22 via vx.WriteRaw; reset on exit.
- Implementations mirroring each widget's real click semantics: block title
  rows hand (tool/agent/bash need a body, compaction needs a summary,
  thinking always, StatusBlock whole-block when Expandable), block bodies and
  user/assistant text (transcript selection), MessageList text, ChatInput
  text, sidebar left border ew-resize, status.Expandable title hand,
  status.ListTile hand when tappable.

---
id: fix-hover-affordance-visual
title: Hover shows no feedback in terminals without OSC 22
status: done
priority: high
task_type: bug
tags:
    - phase1
    - ux
created_at: "2026-08-24T18:43:06.351155Z"
updated_at: "2026-09-02T19:01:40.127246Z"
---

## Body

User reports the mouse cursor does not change on hover. The OSC 22 emission
chain is intact (parser delivers no-button motion, sequence is in the
binary), but OSC 22 only reshapes the pointer in kitty 0.31+/ghostty/foot/
xterm — iTerm2, Terminal.app, Alacritty and tmux ignore it. Add the
terminal-independent affordance opencode uses: a visual hover highlight on
the interactive region under the pointer (theme.BackgroundElement bg).

- components: HoverState{Widget, X, Y} + DrawContext.Hover (propagated by
  WithConstraints) + ApplyHoverRows primitive; ApplyBlockHighlight delegates.
- app: updateHover replaces updatePointerShape — resolves hover target,
  emits OSC 22 on shape change (unchanged), requests a frame when the
  hovered widget or shape changes; draw() publishes &hover.
- widgets light up where the hand shows: tool/agent/bash/compaction/thinking
  title rows (same clickability gates), StatusBlock when expandable,
  status.Expandable title/collapsed, ListTile when tappable.

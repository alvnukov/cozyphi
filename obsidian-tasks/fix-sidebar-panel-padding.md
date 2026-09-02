---
id: fix-sidebar-panel-padding
title: Sidebar text blocks hug the panel frame
status: done
priority: medium
task_type: bug
tags:
    - phase1
    - ux
created_at: "2026-08-24T17:24:03.323755Z"
updated_at: "2026-08-24T17:34:54.974896Z"
---

## Body

Sidebar panel text blocks hug the frame: printPanelLine prints at x=1
truncated to width-2, so content touches the left border glyph at col 0 and
can run into the right border at col width-1; the first row also sits
directly under the labelled top border. Add a one-cell gutter ring: text
prints at 1+pad, truncated to width-2-2*pad; plan wrap width and the context
bar budget shrink to match; the scroll thumb moves into the right gutter so
it stops overwriting the border.

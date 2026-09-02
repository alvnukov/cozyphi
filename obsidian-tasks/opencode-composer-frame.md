---
id: opencode-composer-frame
title: Composer frame in opencode style (left bar panel, inside meta, hints row)
status: done
tags:
    - ui
    - phase-1
    - opencode-parity
created_at: "2026-08-24T07:29:34.78718Z"
updated_at: "2026-08-24T08:27:21.237685Z"
---

## Body

Port the opencode prompt composer look (ground truth
~/src/opencode/packages/tui/src/component/prompt/index.tsx) onto
internal/components/chat.ChatInput:

- Frame: left ┃ bar in the agent/mode color (╹ tail), backgroundElement
  panel fill, paddingLeft/Right 2, paddingTop 1; meta row inside the frame
  bottom ("⏵⏵ build · model"), replacing the rounded border + four
  BorderLabel slots.
- Tail row (╹ + ▀ fade) and a hints row below the frame: cwd muted left,
  usage spans right with a "tab mode · ^k commands" fallback.
- Theme gains BackgroundElement (dark #1e1e1e / light #f5f5f5; legacy
  themes keep terminal default bg).
- ComposerPane setters re-pointed (SetMode/SetModelLabel/SetBranchLabel/
  SetBashBorderActive), footer label seam renamed to usage hints,
  editor.go/pane.go height floors bumped for the extra rows.

Seams: ChatInput.Draw surface geometry, ComposerPane setters, footer
labelComposer interface. Slices red→green per tdd skill.

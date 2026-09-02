---
id: composer-placeholder
title: Composer placeholder parity + height-floor consolidation
status: done
tags:
    - ui
    - phase-1
    - opencode-parity
    - quality
created_at: "2026-08-24T11:45:28.581226Z"
updated_at: "2026-08-24T11:52:18.908059Z"
---

## Body

Close the remaining gaps after the merged opencode composer frame
(daaf06c, finished by the parallel agent):

- Placeholder parity: opencode's prompt shows a muted "Ask anything..."
  when empty and swaps it for "Run a command..." in shell ("!" prefix)
  mode; cozyphi's ChatInput has no Placeholder at all.
- Height-floor consolidation: the composer minimum height logic is
  duplicated in pane.go PreferredHeight (dead — PreferredHeight already
  floors) and editor.go Draw (live clamp). Move the floor to
  ChatInput.MinHeight and consume it from both call sites.
- Delete the dead LabelComposer interface in composer/deps.go (orphaned
  by the SetUsageHints rename).

Seams: ChatInput.Draw surface, ComposerPane setters. Slices red→green.

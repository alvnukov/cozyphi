---
id: hotkey-layout
title: Hotkeys match regardless of keyboard layout (Cyrillic → Latin)
status: done
---

# Hotkeys match regardless of keyboard layout

User report: hotkeys bound to runes (`j`/`k` navigation, `Ctrl+K` palette,
`Ctrl+A` approve, …) stop working when the OS keyboard layout is Russian
(ЙЦУКЕН). The terminal reports the character the active layout produces
(`л` for the physical `k` key), so `ev.Rune == 'k'` never matches.

## Acceptance

- A shared normalization maps the layout's rune back to the Latin QWERTY
  letter of the same physical key (case preserved); Latin runes pass through.
- Kitty keyboard protocol's `codepoint:alternate` pair is parsed — the
  alternate (US-layout key) is kept as `KeyEvent.AltRune`, primary stays the
  typed character for text entry.
- Every rune-bound hotkey compares the normalized rune; text input
  (composer, connect query, permission feedback) keeps the raw rune.
- Integration test: context pane `л`/`о` move the selection exactly like
  `k`/`j`; parser test covers the kitty alternate field.

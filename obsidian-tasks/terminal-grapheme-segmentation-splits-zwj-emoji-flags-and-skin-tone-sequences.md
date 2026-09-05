---
id: terminal-grapheme-segmentation-splits-zwj-emoji-flags-and-skin-tone-sequences
title: Terminal grapheme segmentation splits ZWJ emoji, flags and skin-tone sequences
status: todo
tags:
    - issue
    - feedback
created_at: "2026-09-05T10:47:02.84419Z"
updated_at: "2026-09-05T10:47:02.84419Z"
---

## Body

Observed while implementing composer-editing-profiles: xui/cell/gwidth.go FirstGrapheme extends only Mn/Me/Mc marks; it splits 👩‍💻, 🇫🇷 and 👍🏽, while StringWidth sums rune widths. Composer layout/selection uses that helper, so rendered caret geometry can disagree with terminal grapheme geometry. New editing motions use full grapheme segmentation to preserve text; broader xui rendering correction needs its own targeted tests.

source_repo_path: /Users/zol/src/cozyphi

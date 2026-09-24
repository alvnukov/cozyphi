---
id: terminal-grapheme-segmentation-splits-zwj-emoji-flags-and-skin-tone-sequences
title: Terminal grapheme segmentation splits ZWJ emoji, flags and skin-tone sequences
status: in_progress
tags:
    - issue
    - feedback
branch: task/terminal-grapheme-segmentation-splits-zwj-emoji-flags-and-skin-tone-sequences
worktree_path: .worktrees/terminal-grapheme-segmentation-splits-zwj-emoji-flags-and-skin-tone-sequences
created_at: "2026-09-05T10:47:02.84419Z"
updated_at: "2026-09-24T00:38:50.748968Z"
---

## Body

Observed while implementing composer-editing-profiles: xui/cell/gwidth.go FirstGrapheme extends only Mn/Me/Mc marks; it splits 👩‍💻, 🇫🇷 and 👍🏽, while StringWidth sums rune widths. Composer layout/selection uses that helper, so rendered caret geometry can disagree with terminal grapheme geometry. New editing motions use full grapheme segmentation to preserve text; broader xui rendering correction needs its own targeted tests.

source_repo_path: /Users/zol/src/cozyphi

**Started (2026-09-24).** Взято в работу: воспроизводимый дефект подтверждён скриншотом ленты и кодом — renderCodeBox (internal/components/text/markdown_lines.go) меряет строки через xui.StringWidth, а та суммирует порунные ширины самодельной таблицы (ZWJ=1, модификаторы тона=2, флаги по 1, всё вне 1F300–1FAFF по 1). План: TDD в xui/cell — FirstGrapheme/StringWidth на uniseg-кластерах, затем scoped тесты рендера код-блока.

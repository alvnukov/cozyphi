---
id: fix-overlay-height-wrapping
title: Overlay height estimate ignores wrapping; options unreachable on narrow terminals
status: done
priority: medium
task_type: bug
parent_id: cozyphi-enterprise-code-review
tags:
    - tui
    - overlays
    - review-2026-08
    - sector:tui-shell
created_at: "2026-08-27T16:09:20.869302Z"
updated_at: "2026-08-28T11:49:08.880491Z"
---

## Body

preferredAskHeight (overlays.go:664-692) counts newlines+1 but never wraps detail/header/options at innerW - it computes innerW/method then discards them (_ = method; _ = innerW). paintAskPanel truncates the option list at height-1, so options become unreachable on narrow terminals. question.go:453 optionCount()*2 likewise assumes every option has a description row. Wrap-count the real rows.

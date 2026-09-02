---
id: transcript-readability
title: 'Transcript readability: word wrap, markdown typography, thinking coalescing'
status: done
priority: high
task_type: feature
parent_id: cozyphi-enterprise-code-review
tags:
    - tui
    - phase1-ui
verification_plan:
    - make fmt && go vet ./... && golangci-lint run ./... && go test ./...
    - go test ./internal/components/ ./internal/components/text/ ./internal/session/ -v
    - 'manual: rebuild ~/bin/phi, eyeball a long structured answer'
created_at: "2026-08-23T19:37:36.823634Z"
updated_at: "2026-08-23T19:51:53.276604Z"
---

## Body

Root causes found by pixel/OCR comparison of opencode vs cozyphi screenshots:
1. WrapSpans breaks words mid-grapheme (no word-boundary logic) — "пра|вильный".
2. Flat typography: list items have no hanging indent (wrapped lines glue to the marker), fenced code blocks have no visual chrome, Muted style is double-dimmed (IndexedColor(8)+Dim / 245+Dim → nearly invisible), bold renders barely brighter than body in Terminal theme → assistant answers read as one gray wall (screenshot: 99% of text pixels are a single #bbbbbb).
3. Consecutive thinking blocks render as N separate "• Thinking •" rows (7 in the sample screenshot); session.Project emits one ItemThinking per BlockThinking and never coalesces.

Fix plan (public seams, tests beside code):
- components.WrapSpans: word-aware greedy wrap (break at last word boundary; grapheme hard-break fallback for overlong words; CJK unaffected).
- session.Project: coalesce consecutive ItemThinking rows (within a message and across adjacent messages) into one.
- text.RenderMarkdown: hanging indent for list continuation lines; fenced code blocks get a left rule + lang caption.
- components themes: Muted drops the extra Dim (Terminal idx8, Dark 245) so markers/meta stay readable.

## Verification Plan

1. make fmt && go vet ./... && golangci-lint run ./... && go test ./...
2. go test ./internal/components/ ./internal/components/text/ ./internal/session/ -v
3. manual: rebuild ~/bin/phi, eyeball a long structured answer

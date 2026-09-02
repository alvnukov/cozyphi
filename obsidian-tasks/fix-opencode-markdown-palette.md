---
id: fix-opencode-markdown-palette
title: 'Markdown/syntax палитра мимо opencode.json: радуга вместо фиолетовых заголовков и зелёного кода'
status: done
priority: high
task_type: bug
parent_id: cozyphi-convenience-program
tags:
    - ui
    - theme
    - phase1-ui
    - opencode-parity
verification_plan:
    - go test ./internal/components/... ./internal/tui/... — new role tests red→green, legacy theme tests unchanged
    - 'gate: fmt, vet, lint, full test'
    - 'manual: transcript with headings/code/links matches opencode colors (purple headings, green inline code)'
created_at: "2026-08-24T06:24:06.530491Z"
updated_at: "2026-08-24T06:43:02.274245Z"
---

## Body

User report: "цветовая палитра вообще не из опенкода". Root cause: ui-opencode-theme ported only the 12 semantic roles verbatim from opencode.json; markdown and code-highlight colors were improvised by reusing Warning/Success/ToolName (H1 green, H2 blue, H3 orange, inline code orange, keyword blue-bold, no-lang code boxes orange). Real opencode (cloned to ~/src/opencode, packages/tui/src/theme/assets/opencode.json + theme/index.ts textmate scopes) uses dedicated roles: markdownHeading #9d7cd8 bold (H1 +underline, all levels same color), markdownStrong #f5a742, markdownEmph/markdownBlockQuote #e5c07b italic, markdownCode #7fd88f, markdownLinkText #56b6c2 underline, markdownListItem #fab283, markdownListEnumeration #56b6c2, markdownCodeBlock #eeeeee; syntax: keyword #9d7cd8, function #fab283, variable #e06c75, string #7fd88f, number #f5a742, type #e5c07b, operator #56b6c2, punctuation #eeeeee, comment #808080.

Fix: extend components.Theme with Markdown and Syntax role groups (values verbatim from opencode.json for opencode/opencode-light; derived to preserve current look for Dark/Darcula/Pink/Terminal), remap text renderer (markdown.go/markdown_lines.go) and chromaStyle to consume the roles, prose path highlighting becomes underline-only (no orange repaint). Ground truth: ~/src/opencode clone.

## Verification Plan

1. go test ./internal/components/... ./internal/tui/... — new role tests red→green, legacy theme tests unchanged
2. gate: fmt, vet, lint, full test
3. manual: transcript with headings/code/links matches opencode colors (purple headings, green inline code)

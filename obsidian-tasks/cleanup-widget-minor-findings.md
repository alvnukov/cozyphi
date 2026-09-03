---
id: cleanup-widget-minor-findings
title: 'Widget panes: minor fixes bundle from 2026-08 review'
status: done
priority: low
task_type: chore
parent_id: cozyphi-enterprise-code-review
tags:
    - cleanup
    - tui
    - review-2026-08
    - sector:tui-shell
created_at: "2026-08-27T16:09:20.903103Z"
updated_at: "2026-08-28T11:49:08.882895Z"
---

## Body

1) Mapper.expanded (transcript/mapper.go:59-70) never pruned - LoadReplay/ResetSubagents clear ids but not the map; clear alongside t.listIDs=nil. 2) HideCompleters == CloseMentionSlash byte-identical (composer/pane.go:180-190 vs 243-253), both on the Input interface - delete one. 3) chrome-glyph set duplicated: chat/chat_input.go:760 isComposerChrome and components/selection.go:199 isSelectionChrome - hoist one predicate. 4) ctxpane/pane.go:672-679 indexLabel is O(n^2) per draw and misnumbers rows when one entry spans several items - pass the known idx down. 5) command_palette.go:226 comment promises close-on-click-outside; mouse case (:354-357) only consumes - fix comment or add hit-testing. 6) text/markdown_stream.go:69-71: any '[' permanently disables incremental layout for the rest of the message - match reference definitions (^\s{0,3}\[[^]]+\]:) instead.

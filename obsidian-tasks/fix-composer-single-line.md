---
id: fix-composer-single-line
title: Collapse composer to one line, keep multiline growth
status: done
priority: medium
task_type: bug
tags:
    - tui
    - composer
created_at: "2026-08-25T05:10:00.000000Z"
updated_at: "2026-08-25T05:10:00.000000Z"
---

## Body

The chat composer is too tall: `newChatInput` sets `MinBodyRows: 3`, so the
editor area keeps three rows even when empty. Reduce the default to one row;
`ChatInput.bodyRows` already grows the editor with content up to
`MaxBodyRows: 8`, so multiline input still expands.

source_repo_path: /Users/zol/src/cozyphi
resolved_by: ff57656 fix(tui): collapse composer to one line when empty

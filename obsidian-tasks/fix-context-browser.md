---
id: fix-context-browser
title: Context viewer/editor (/context browser)
status: done
priority: medium
task_type: feature
tags:
    - tui
    - session
    - context
created_at: "2026-08-25T07:30:00.000000000Z"
updated_at: "2026-08-25T07:30:00.000000000Z"
---

## Body

Users cannot see or shape what the model actually receives: the context is
invisible until compaction fires. Add a full-screen `/context` browser (the
`sessions` pattern): header with usage/window/threshold, one row per context
entry (role, token estimate, cumulative share, preview), scroll + selection,
and two actions — compact now, and trim-from-here (append-only compaction
entry with a user note instead of an LLM summary, keeping the audit log intact).

source_repo_path: /Users/zol/src/cozyphi
resolved_by: 0a6c6a1 feat(tui): full-screen context browser with /context command

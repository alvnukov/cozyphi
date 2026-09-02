---
id: reasoning-compaction-markdown
title: Render reasoning and compaction bodies as Markdown
status: done
priority: high
task_type: feature
tags:
    - tui
    - markdown
    - phase1
created_at: "2026-08-24T21:00:00.000000Z"
updated_at: "2026-08-24T21:00:00.000000Z"
---

## Body

ThinkingBlock and CompactionBlock currently render their expanded bodies as
one dim, muted plain-text span: headings, lists, code, and emphasis all show
up as a grey wall of text. Route both bodies through the shared Markdown
renderer (like AssistantBlock and AgentBlock do) so reasoning and compaction
summaries get normal themed Markdown typography.

source_repo_path: /Users/zol/src/cozyphi

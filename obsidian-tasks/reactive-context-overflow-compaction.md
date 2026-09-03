---
id: reactive-context-overflow-compaction
title: Reactive compaction on provider context overflow
status: done
priority: high
task_type: feature
tags:
    - agent
    - compaction
    - phase1
created_at: "2026-08-24T19:30:00.000000Z"
updated_at: "2026-08-24T19:30:00.000000Z"
---

## Body

When a provider rejects a request because the context window is exceeded
(Anthropic `prompt is too long`, OpenAI `context_length_exceeded`, etc.), the
turn currently dies with the raw API error. Compact the session and retry the
request once instead, mirroring OpenCode's `compactAfterOverflow`. Non-overflow
errors keep their existing fail-fast behavior.

Also strengthen the compaction update-summary merge rules with OpenCode's
"conversation wins on conflict" and "drop only what is finished" directives.

source_repo_path: /Users/zol/src/cozyphi

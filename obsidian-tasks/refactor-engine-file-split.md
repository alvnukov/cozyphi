---
id: refactor-engine-file-split
title: engine.go (1494 lines) mixes six responsibilities; extract compaction/usage and memory recall
status: done
priority: medium
task_type: refactor
parent_id: cozyphi-enterprise-code-review
tags:
    - architecture
    - review-2026-08
    - sector:agent-context
created_at: "2026-08-27T16:09:20.791848Z"
updated_at: "2026-08-27T23:27:35.544052Z"
---

## Body

One file owns client/tool rebinding, plan management (7 methods), memory recall, compaction + token accounting, streaming/event emission, skills instructions. Extract contextStats/currentContextUsage/runCompaction/estimateContextTokens and the memory-recall block into their own files/types beside the engine. Also: contextStats re-implements estimateContextTokens inline (engine.go:1123-1131 vs 1196-1202) - one helper; the speculative RunUntil godoc sits on maybeCompact (engine.go:946-958); Loop duplicates the skills-instruction prepend at engine.go:693-700 and 818-825 - extract composeUserPrompt.

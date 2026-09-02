---
id: feature-context-tool
title: 'context tool: usage report + self-serve compaction handle'
status: done
priority: high
task_type: feature
parent_id: cozyphi-enterprise-code-review
tags:
    - agent
    - tools
    - compaction
created_at: "2026-08-23T16:52:19.131768Z"
updated_at: "2026-08-23T17:10:32.903986Z"
---

## Body

Add a model-facing `context` tool: quantitative usage report (context tokens + serialized KB + window + compact threshold + recommended flag; never conversation content) and an explicit compact action. Compaction request applies at the tool-round boundary in Engine.Loop (after tool results are appended) so assistant/tool-call pairing is never split; transcript stays append-only. Guards: already-scheduled error, nothing-to-compact error. Permission: ActionContext → Allow (no external effects). Tool added in Engine.buildToolList so main engine and sub-agents both get it.

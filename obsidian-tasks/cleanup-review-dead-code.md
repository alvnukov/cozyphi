---
id: cleanup-review-dead-code
title: 'Dead code from 2026-08 review: executor/session/llm/lsp/commands leftovers'
status: done
priority: low
task_type: refactor
parent_id: cozyphi-enterprise-code-review
tags:
    - cleanup
    - review-2026-08
    - sector:tail
created_at: "2026-08-27T16:09:20.904633Z"
updated_at: "2026-08-28T12:13:12.345815Z"
---

## Body

No production callers for: Executor.Run (executor.go:97-104); Session.AddUser/AddAssistant/AddFinalAssistant/Len/LastID (session.go:304-343); llm/client stores anthropic from isAnthropicProvider but never reads it (client/client.go:23,81-83) - delete both; lsp graceFrom discards ctx and returns a constant (manager.go:171-174) - drop the param; commands FilterSlashCommands/LookupSlashInsert/PaletteCommands (builtins.go:260,322,364) each rebuild a registry. Layout-zoo dead toolkit (~900 lines in components/layout, components/input, status.go:306-583) is already tracked by refactor-delete-dead-code - this task excludes it. Verify each with references search before deleting.

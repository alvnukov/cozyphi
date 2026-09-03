---
id: fix-permission-extract-question-mcp
title: 'permission extract: question and mcp_* fall into unknown-action Ask; MCP dead in autopilot'
status: done
priority: high
task_type: bug
parent_id: cozyphi-enterprise-code-review
tags:
    - security
    - permissions
    - mcp
    - review-2026-08
created_at: "2026-08-27T16:09:20.796218Z"
updated_at: "2026-08-27T16:20:04.319805Z"
---

## Body

internal/permission/extract.go:153 default: sends question and mcp_list/mcp_inspect/mcp_call to Ask with 'unknown action' (gate.go:87). Interactive: every question call shows a permission overlay before the question UI; headless/autopilot folds Ask->Deny (gate.go:231-235) so question can never run and MCP is dead, while plangate exempts question and StepIntegrate expects mcp_call to work. Fix: explicit cases - question->Allow (it IS the ask), mcp_list/mcp_inspect->Allow (read-only), mcp_call->Ask. Extends existing task fix-mcp-call-permission-default. Same file: extract.go:69,80 swallow json.Unmarshal errors for grep/find args - malformed args judge as path '.' (allowed); propagate like sibling cases.

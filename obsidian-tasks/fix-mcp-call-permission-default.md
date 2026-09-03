---
id: fix-mcp-call-permission-default
title: 'mcp_call падает в default-ветку пермишенов: интерактивно вечный Ask, headless отказ'
status: done
priority: medium
task_type: bug
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - mcp_call получает осмысленную политику
    - headless-путь не ломает MCP
verification_plan:
    - ручная проверка mcp_call в TUI и phi run
created_at: "2026-08-23T15:17:22.110002Z"
updated_at: "2026-09-02T19:01:40.129504Z"
---

## Body

internal/permission/extract.go:107-109 default: req.Action = Action(toolName); gate.go:72-73 возвращает Ask 'unknown action'. Интерактивно каждый mcp_call спрашивает; headless (Ask:nil, cmd/run.go:96) — отказ, MCP под phi run неиспользуем, хотя pool загружен. Новый мутирующий тул с незнакомым именем получает Ask, никогда Deny. Полное решение — дескрипторы пермишенов у тулов (refactor-tool-owned-permissions).

## Acceptance Criteria

- mcp_call получает осмысленную политику
- headless-путь не ломает MCP

## Verification Plan

1. ручная проверка mcp_call в TUI и phi run

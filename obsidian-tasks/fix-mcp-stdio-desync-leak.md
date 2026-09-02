---
id: fix-mcp-stdio-desync-leak
title: 'stdio MCP: рассинхрон ответов и утечка горутины после таймаута'
status: done
priority: medium
task_type: bug
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - ответы матчатся по ID, таймаут закрывает сессию
    - тест таймаут-then-call
verification_plan:
    - go test ./internal/mcp/...
created_at: "2026-08-23T15:17:22.108603Z"
updated_at: "2026-08-27T20:01:08.339456Z"
---

## Body

internal/mcp/stdio.go:59-83: ридер-горутина переживает select-таймаут и паркуется в stdout.ReadBytes навсегда; readResponse (:167-183) не сопоставляет rpc.ID с запросом — следующий вызов сессии съедает предыдущий поздний ответ; session.ready остаётся true. Каждый вызов после таймаута возвращает чужой результат до рестарта.

## Acceptance Criteria

- ответы матчатся по ID, таймаут закрывает сессию
- тест таймаут-then-call

## Verification Plan

1. go test ./internal/mcp/...

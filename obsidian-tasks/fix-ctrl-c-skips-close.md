---
id: fix-ctrl-c-skips-close
title: 'Ctrl+C выходит без Controller.Close: хуки, jobs, mcp-pool текут'
status: done
priority: high
task_type: bug
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - Ctrl+C триггерит session_shutdown и закрывает jobs/mcp
    - тест пути выхода
verification_plan:
    - ручная проверка хука при Ctrl+C
created_at: "2026-08-23T15:17:22.107223Z"
updated_at: "2026-08-23T15:57:46.93977Z"
---

## Body

internal/components/app/app.go:186-188 перехватывает Ctrl+C и возвращает quit до dispatch; единственный вызов ctrl.Close() — замыкание в editor.go:214-218 (CtrlC-ветка композера), недостижимое. session_shutdown-хук, jobs.Close, mcpPool.Close (controller.go:607-621) не выполняются на самом частом пути выхода. Фикс: Close по пути выхода из app.Run.

## Acceptance Criteria

- Ctrl+C триггерит session_shutdown и закрывает jobs/mcp
- тест пути выхода

## Verification Plan

1. ручная проверка хука при Ctrl+C

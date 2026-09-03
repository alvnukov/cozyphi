---
id: fix-agent-spawn-workdir-escape
title: 'agent_spawn: workdir модели становится границей записи ребёнка без спроса'
status: done
priority: high
task_type: bug
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - workdir вне workspace родителя либо отклоняется, либо требует подтверждения
    - тест на escape-сценарий
verification_plan:
    - go test ./internal/job/... ./internal/agent/...
created_at: "2026-08-23T15:17:22.105271Z"
updated_at: "2026-08-23T16:28:19.244827Z"
---

## Body

Готово (interim, до refactor-child-factory). Корень: ExtractAt не несёл workdir в Paths, а StaticGate на ActionAgent отдавал Allow безусловно — workdir ребёнка молча становился его write-границей. Фикс (80eeb05): (1) extract.go — agent_spawn кладёт workdir в req.Paths (абс. против cwd сессии); (2) gate.go checkAgent — при WorkspaceOnlyWrites путь вне workspace → Ask (в неинтерактивных режимах фолдится в Deny), без workdir → Allow; (3) agenttool/agent.go — относительный workdir резолвится в абсолютный против родительского cwd до SpawnRequest (тот же AbsCleanAt, что в гейте). Тесты: трассер TestLoopAgentSpawnWorkdirEscapeAsks (Ask, 0 джобов, ToolRejected), TestExtractAgentSpawnWorkdirBecomesPath, TestGateAgentSpawnWorkdirEscapeAsks, TestAgentToolsSpawnResolvesWorkdirAgainstParent. Гейты: make fmt-check lint test зелёные.

## Acceptance Criteria

- workdir вне workspace родителя либо отклоняется, либо требует подтверждения
- тест на escape-сценарий

## Verification Plan

1. go test ./internal/job/... ./internal/agent/...

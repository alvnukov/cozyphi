---
id: refactor-child-factory
title: 'Суб-агенты: единая child-factory с проверкой confinement'
status: done
priority: high
task_type: refactor
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - confinement-проверка в одном месте
    - одна форма SpawnRequest, один парсер
verification_plan:
    - go test ./internal/agent/... ./internal/job/...
created_at: "2026-08-23T15:17:22.118226Z"
updated_at: "2026-08-23T16:43:11.036695Z"
---

## Body

Сделано в 0a06b7c (feat/ui-render-pipeline). Спавн консолидирован:

- job.SpawnRequest/Meta несут ParentWorkspace; Manager.Spawn валидирует резолв workdir внутрь родительского workspace (синхронная ошибка, модель может самокорректироваться) и хранит его абсолютным.
- EngineRunner.buildChild — единственная точка сборки ребёнка (role spec, гейт, тулы, сессия, промпт); re-assert confinement fail-closed — ручной/чужой Spawn-вызов не расширит границу через meta.
- Родительский permission-гейт больше не спецкейсит agent_spawn: интеримный Ask (80eeb05, не выпущен) заменён spawn-валидацией; «одобренный эскейп» удалён как гипотетическая фича (ADR в теле коммита).
- Удалены мёртвые job.SpawnArgs/HandleSpawn (второй парсер того же JSON) и поля EngineRunner.Gate/Tools (никем не сеттились).

Оба AC выполнены: confinement-проверка в одном месте (validate + assert на сборке гейта, один предикат permission.InWorkspace); одна форма SpawnRequest, один парсер (agenttool). Верификация: make fmt-check lint test зелёные; happ diagnostics чистые. Тесты: spawn_workspace_test.go (резолв/дефолт/эскейп), TestEngineRunnerRejectsEscapingWorkdir (rogue meta), tracer TestLoopAgentSpawnWorkdirEscapeFailsSync (без Ask, ToolError, 2 запроса).

## Acceptance Criteria

- confinement-проверка в одном месте
- одна форма SpawnRequest, один парсер

## Verification Plan

1. go test ./internal/agent/... ./internal/job/...

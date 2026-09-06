---
id: developer-mode-mcp
title: 09 — Наблюдать MCP без подключения и раскрытия конфигурации
status: done
priority: medium
model_level: medium
task_type: feature
parent_id: developer-mode-readonly
tags:
    - developer-mode
acceptance_criteria:
    - 'integrations.mcp доступен через catalog/snapshot/explain: configured/loaded/running, source precedence и workspace scope.'
    - Различаются disabled, not configured, not loaded, connected, closed и безопасный load/runtime failure там, где owner их знает.
    - Чтение не запускает сервер, соединение, health probe или discovery; информация только из существующего состояния.
    - Не выдаются env values, raw arguments/commands, credentials, схемы MCP tools или неочищенные ошибки.
    - Отсутствующий pool не делает весь snapshot ошибочным; TUI/headless используют одинаковую модель состояний.
verification_plan:
    - Fixtures source precedence и configured/loaded/running/error/closed.
    - Spies connect/discover/run фиксируют ноль вызовов от collector.
    - Sentinel в args/env/URL/errors и server schema отсутствует в output/audit; частичный snapshot с nil pool.
created_at: "2026-09-06T09:08:35.742227Z"
updated_at: "2026-09-06T14:21:33.189598Z"
---

## Body

**Что построить.** Read-only MCP диагностика через integrations-категорию harness. Читать epic developer-mode-readonly.

**Blocked by:** developer-mode-headless.

**Scope.** Существующий pool и безопасная provenance imported/global/project конфигурации. Не менять precedence или доверие к источникам, не строить второй MCP loader. Состояния брать у владельца под его синхронизацией, не вычислять connected по наличию config. Unknown data — unavailable с безопасной причиной. Данные MCP остаются за meta-tools; developer mode не открывает схемы серверных инструментов.

**Работа.** Task worktree; тесты на локальных fixtures/spies без реальных servers; closeout по epic.

## Acceptance Criteria

- integrations.mcp доступен через catalog/snapshot/explain: configured/loaded/running, source precedence и workspace scope.
- Различаются disabled, not configured, not loaded, connected, closed и безопасный load/runtime failure там, где owner их знает.
- Чтение не запускает сервер, соединение, health probe или discovery; информация только из существующего состояния.
- Не выдаются env values, raw arguments/commands, credentials, схемы MCP tools или неочищенные ошибки.
- Отсутствующий pool не делает весь snapshot ошибочным; TUI/headless используют одинаковую модель состояний.

## Verification Plan

1. Fixtures source precedence и configured/loaded/running/error/closed.
2. Spies connect/discover/run фиксируют ноль вызовов от collector.
3. Sentinel в args/env/URL/errors и server schema отсутствует в output/audit; частичный snapshot с nil pool.

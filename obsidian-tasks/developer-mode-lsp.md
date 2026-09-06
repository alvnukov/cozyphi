---
id: developer-mode-lsp
title: 10 — Наблюдать LSP без запуска, синхронизации и diagnostics
status: blocked
priority: medium
model_level: medium
task_type: feature
parent_id: developer-mode-readonly
tags:
    - developer-mode
acceptance_criteria:
    - integrations.lsp показывает configured/installed/running, workspace, поддерживаемые операции и безопасный source/state.
    - Не вызываются server start, download, workspace sync, Query или diagnostics ради самого снимка.
    - Сохраняются trust/validation rules LSP config; developer mode не загружает файл обходным способом.
    - Неподдержанный язык, выключенный manager, закрытие и неизвестная freshness представлены явно.
    - DTO не раскрывает secret-bearing args/env/raw process errors; snapshots detached и безопасны при смене состояния.
verification_plan:
    - 'Fixture manager: absent/configured/installed/running/closed/unsupported, корректный workspace.'
    - Spies start/download/query/sync не вызываются при harness чтении.
    - Race-проверка затронутого status path при shutdown и tests secret redaction.
created_at: "2026-09-06T09:08:35.744764Z"
updated_at: "2026-09-06T09:08:35.744764Z"
---

## Body

**Что построить.** Метаданные LSP runtime доступны через harness integrations без воздействия на language server. Читать epic developer-mode-readonly.

**Blocked by:** developer-mode-headless.

**Фиксированные решения.** Наблюдать manager и его known status; не пользоваться Query как shortcut к status, если это может запускать server или синхронизацию. Installed можно сообщать по уже известному состоянию владельца; иначе unavailable, а не probe с side effects. Diagnostics freshness не обещается без наблюдения владельца. Не менять LSP ownership/lifetime.

**Работа.** Task worktree; профильные unit/integration tests без настоящего language server; closeout по epic.

## Acceptance Criteria

- integrations.lsp показывает configured/installed/running, workspace, поддерживаемые операции и безопасный source/state.
- Не вызываются server start, download, workspace sync, Query или diagnostics ради самого снимка.
- Сохраняются trust/validation rules LSP config; developer mode не загружает файл обходным способом.
- Неподдержанный язык, выключенный manager, закрытие и неизвестная freshness представлены явно.
- DTO не раскрывает secret-bearing args/env/raw process errors; snapshots detached и безопасны при смене состояния.

## Verification Plan

1. Fixture manager: absent/configured/installed/running/closed/unsupported, корректный workspace.
2. Spies start/download/query/sync не вызываются при harness чтении.
3. Race-проверка затронутого status path при shutdown и tests secret redaction.

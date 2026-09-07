---
id: developer-mode-coverage
title: 16 — Проверить полноту, секреты и read-only поведение developer mode
status: todo
priority: medium
model_level: medium
task_type: test
parent_id: developer-mode-readonly
tags:
    - developer-mode
acceptance_criteria:
    - 'Матрица учитывает все supported config fields, CLI overrides, известные ENV overrides, imports/session overrides и существенные встроенные лимиты: каждое сопоставлено с catalog либо явно обоснованным исключением.'
    - В финальном catalog нет временного not_implemented из 01; unavailable допустим только как реальное состояние/обоснованное ограничение, не замена неподключённого collector.
    - Сквозная TUI/headless матрица подтверждает флаг, отсутствие включения через config/ENV/resume/UI, отсутствие наследования children и прямой отказ без capability.
    - Sentinel secrets в keys/URL/args/env/paths/errors/contents не появляются в tool results, error paths, transcript и audit.
    - Spy adapters подтверждают отсутствие дополнительных reload/probe/start/scan/write; штатные pre/post hooks и evidence самого tool call не ошибочно считаются collector side effects.
    - Concurrency/cancellation/truncation/scope tests проверяют общий interface с несколькими owners; выполняются только scoped проверки.
    - Новые крупные недоделки/баги оформлены отдельными medium/low задачами и блокируют приёмку, а не незаметно реализуются в этом тестовом билете.
verification_plan:
    - 'Составить и автоматически проверить coverage inventory: категории, поля, sources, exceptions; сопоставить с collectors.'
    - Запустить scoped end-to-end matrix через настоящий harness interface на fixture TUI/headless engines.
    - Проверить security sentinel output channels, side-effect spies, scope/cancellation/races; приложить точные команды и результаты к задаче.
created_at: "2026-09-06T09:09:49.306932Z"
updated_at: "2026-09-07T01:18:13.215646Z"
---

## Body

**Что построить.** Приёмочная доказательная матрица полного read-only developer mode, а не новая реализация всех collectors. Читать epic developer-mode-readonly и результаты задач 02–15.

**Blocked by:** developer-mode-tui, developer-mode-model, developer-mode-providers, developer-mode-permissions, developer-mode-tools, developer-mode-context, developer-mode-plan, developer-mode-mcp, developer-mode-lsp, developer-mode-hooks, developer-mode-agents, developer-mode-storage, developer-mode-ui, developer-mode-diagnostics.

**Фиксированные решения.** Каждая collector-задача уже поставляет собственные тесты; здесь проверяется совместный контракт и полнота инвентаризации, не пишется второй набор внутренних unit tests. Проверяемая coverage table должна ловить незарегистрированные новые настройки через сопоставление с известными loaders/schema/flags, а runtime-only limits иметь явный перечень. Не требовать reflection dump конфигов. На исходниках архитектуру массово не менять. Допустимые исключения объясняют отсутствие информации; нельзя закрыть обещание полноты, назвав все ещё не реализованные поля unavailable.

**Работа.** Task worktree. Запускать только пакеты/сценарии developer mode и затронутых owners, без make test/lint всего repo. Fixture runtime без реальных credentials, провайдеров, servers и OS notifications. Closeout по epic.

## Acceptance Criteria

- Матрица учитывает все supported config fields, CLI overrides, известные ENV overrides, imports/session overrides и существенные встроенные лимиты: каждое сопоставлено с catalog либо явно обоснованным исключением.
- В финальном catalog нет временного not_implemented из 01; unavailable допустим только как реальное состояние/обоснованное ограничение, не замена неподключённого collector.
- Сквозная TUI/headless матрица подтверждает флаг, отсутствие включения через config/ENV/resume/UI, отсутствие наследования children и прямой отказ без capability.
- Sentinel secrets в keys/URL/args/env/paths/errors/contents не появляются в tool results, error paths, transcript и audit.
- Spy adapters подтверждают отсутствие дополнительных reload/probe/start/scan/write; штатные pre/post hooks и evidence самого tool call не ошибочно считаются collector side effects.
- Concurrency/cancellation/truncation/scope tests проверяют общий interface с несколькими owners; выполняются только scoped проверки.
- Новые крупные недоделки/баги оформлены отдельными medium/low задачами и блокируют приёмку, а не незаметно реализуются в этом тестовом билете.

## Verification Plan

1. Составить и автоматически проверить coverage inventory: категории, поля, sources, exceptions; сопоставить с collectors.
2. Запустить scoped end-to-end matrix через настоящий harness interface на fixture TUI/headless engines.
3. Проверить security sentinel output channels, side-effect spies, scope/cancellation/races; приложить точные команды и результаты к задаче.

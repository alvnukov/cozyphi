---
id: developer-mode-headless
title: 01 — Включить минимальный read-only harness в headless
status: done
priority: medium
model_level: medium
task_type: feature
parent_id: developer-mode-readonly
tags:
    - developer-mode
acceptance_criteria:
    - cozyphi run --developer-mode предоставляет harness catalog/snapshot/explain; без флага нет регистрации и прямой вызов отвергается.
    - Из реального runtime доступны версия/build, headless mode, безопасная workspace/session identity и стартовое developer-состояние; explain показывает CLI/default как источник.
    - Catalog сразу перечисляет все категории epic; ещё не подключённые категории имеют явный unavailable/not_implemented, не фиктивные значения.
    - Общий DTO различает слои и состояния epic; false/0 не превращаются в absent. Snapshot detached, ограничен по размеру, имеет explicit truncation/partial; параметры action/category/key валидируются.
    - Модуль диагностики не зависит от TUI; harness регистрируется до plan-step injection и проходит существующие hooks/gates без новой возможности обхода.
    - Разрешение CLI не сохраняется в config/session; никаких developer ENV overrides, writes, сетевых/процессных probes. Добавлены тесты и Unreleased запись.
verification_plan:
    - 'Тесты CLI parsing/help: флаг есть/нет, неизвестные аргументы, config/ENV не включают режим.'
    - 'Тесты через interface harness: catalog, snapshot runtime, explain, invalid args, detached snapshot, bounded output и sentinel secret.'
    - 'Headless engine integration: обычный tool loop/plan gates сохранены, без флага и при direct call отказ; без реального провайдера/сети.'
created_at: "2026-09-06T09:05:48.890229Z"
updated_at: "2026-09-06T10:09:54.718775Z"
---

## Body

**Что построить.** Первый проверяемый сквозной путь: старт headless с --developer-mode → стартовые read-полномочия → UI-независимый модуль диагностики → нативный harness → ограниченный ответ runtime. Общий контракт и запреты находятся в epic developer-mode-readonly; прочитать его до реализации.

**Blocked by:** None — can start immediately.

**Фиксированные решения.** Interface только catalog/snapshot/explain. Создать малый общий DTO и именованные лимиты ответа; не строить plugin framework или универсальный settings exporter. Для bounded output предпочтительны безопасное усечение по полям с explicit truncation и уточнение category/key, не сложный pagination protocol. Catalog не обещает работу ещё не подключённых collectors. Runtime capability — явный параметр сборки, не mutable setting. Прямой вызов инструмента без capability возвращает безопасный отказ даже при ошибке регистрации. Не переносить состояние из его владельцев в диагностику.

**Scope.** Только headless/runtime и достаточная инфраструктура для следующей категории. Не реализовывать TUI, остальные collectors или специальную защиту запуска. Временные not_implemented должны быть видны и затем закрыты задачей developer-mode-coverage. Все будущие collectors используют те же DTO и правила redaction. Формат source — безопасные метаданные, не произвольный raw string.

**Работа.** Код в отдельном task worktree; commit/merge/ledger/cleanup как в epic. Проверки только затронутых пакетов.

## Acceptance Criteria

- cozyphi run --developer-mode предоставляет harness catalog/snapshot/explain; без флага нет регистрации и прямой вызов отвергается.
- Из реального runtime доступны версия/build, headless mode, безопасная workspace/session identity и стартовое developer-состояние; explain показывает CLI/default как источник.
- Catalog сразу перечисляет все категории epic; ещё не подключённые категории имеют явный unavailable/not_implemented, не фиктивные значения.
- Общий DTO различает слои и состояния epic; false/0 не превращаются в absent. Snapshot detached, ограничен по размеру, имеет explicit truncation/partial; параметры action/category/key валидируются.
- Модуль диагностики не зависит от TUI; harness регистрируется до plan-step injection и проходит существующие hooks/gates без новой возможности обхода.
- Разрешение CLI не сохраняется в config/session; никаких developer ENV overrides, writes, сетевых/процессных probes. Добавлены тесты и Unreleased запись.

## Verification Plan

1. Тесты CLI parsing/help: флаг есть/нет, неизвестные аргументы, config/ENV не включают режим.
2. Тесты через interface harness: catalog, snapshot runtime, explain, invalid args, detached snapshot, bounded output и sentinel secret.
3. Headless engine integration: обычный tool loop/plan gates сохранены, без флага и при direct call отказ; без реального провайдера/сети.

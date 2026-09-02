---
id: claude-onboarding-and-memory-curation
title: 'CLI: cozyphi claude onboard (AGENTS.md для репозитория) и cozyphi memory curate'
status: todo
priority: low
model_level: medium
task_type: feature
parent_id: claude-consult-tool
tags:
    - claude
    - cli
    - memory
    - onboarding
acceptance_criteria:
    - onboard пишет предложение в новый файл и никогда не трогает существующий AGENTS.md; curate без --apply ничего не меняет, с --apply применяет только через операции памяти.
    - Обе команды печатают usage/cost; make fmt-check lint test rc=0.
verification_plan:
    - go test ./cmd/... ./internal/memory/... -run 'Claude|Curate' -race.
    - make fmt-check lint test.
created_at: "2026-09-02T05:13:31.876283Z"
updated_at: "2026-09-02T05:13:31.876283Z"
---

## Body

**Цель.** Разовые инвестиции токенов Claude, после которых дешёвая модель ошибается реже.

**cozyphi claude onboard.** Один research-проход по репозиторию (Tools Read,Grep,Glob, AddDirs=workspace) с deliverable «AGENTS.md: назначение, layout, quality bar, инварианты, команды проверки»; результат пишется в AGENTS.claude.md (или как diff к существующему AGENTS.md) — никогда не перезаписывает; печатает usage/cost.

**cozyphi memory curate.** Отправляет корпус фактов (каталог + тела, в бюджете брифа) с deliverable «merge/rewrite/demote предложения по имени файла»; по умолчанию dry-run с печатью предложений и diff; --apply применяет через существующие операции памяти (write/forget/demote), ничего не удаляя.

**Тесты.** разбор флагов, dry-run без записей, --apply на временном каталоге памяти, отказ по бюджету на большом корпусе с подсказкой.

**Зависит от:** claude-modes-and-brief. Фаза 2.

## Acceptance Criteria

- onboard пишет предложение в новый файл и никогда не трогает существующий AGENTS.md; curate без --apply ничего не меняет, с --apply применяет только через операции памяти.
- Обе команды печатают usage/cost; make fmt-check lint test rc=0.

## Verification Plan

1. go test ./cmd/... ./internal/memory/... -run 'Claude|Curate' -race.
2. make fmt-check lint test.

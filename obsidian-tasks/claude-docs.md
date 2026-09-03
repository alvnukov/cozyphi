---
id: claude-docs
title: doc/claude.md, README, project-layout, инвариант в AGENTS.md, CHANGELOG
status: todo
priority: medium
model_level: medium
task_type: docs
parent_id: claude-consult-tool
tags:
    - claude
    - docs
acceptance_criteria:
    - doc/claude.md покрывает конфиг, режимы, бриф, вложения, треды, тормоза, стоимость и карту кода; README/project-layout/AGENTS.md/CHANGELOG обновлены.
    - Лимиты в документации совпадают с константами в коде.
    - make fmt-check lint test rc=0.
verification_plan:
    - 'Прочитать doc/claude.md против кода: каждый флаг/лимит/путь существует.'
    - make fmt-check lint test.
created_at: "2026-09-02T05:13:31.876016Z"
updated_at: "2026-09-02T05:13:31.876016Z"
---

## Body

**Что сделать.** doc/claude.md по образцу doc/watch.md: зачем (консультант, не sub-agent), конфиг ~/.cozyphi/claude.json с примером, таблица режимов с профилями и ценой, шаблон брифа, вложения, треды, тормоза, что показывает результат, где живёт guidance (system prompt vs описание тулы), карта кода. README: пункт в Highlights и строка в Documentation; doc/project-layout.md: строки internal/claude и internal/tools/claudetool; AGENTS.md: инвариант «Claude consults: brief, never the session; read-only modes allow, implement asks and runs in a worktree; children and sub-agents get no tool; usage lands in the ledger». CHANGELOG: одна связная запись под [Unreleased], заменяющая построчные записи детей, если они дублируют друг друга. Проверить, что цифры лимитов в документации совпадают с константами.

**Зависит от:** всех остальных детей тира A/B (пишется последней).

## Acceptance Criteria

- doc/claude.md покрывает конфиг, режимы, бриф, вложения, треды, тормоза, стоимость и карту кода; README/project-layout/AGENTS.md/CHANGELOG обновлены.
- Лимиты в документации совпадают с константами в коде.
- make fmt-check lint test rc=0.

## Verification Plan

1. Прочитать doc/claude.md против кода: каждый флаг/лимит/путь существует.
2. make fmt-check lint test.

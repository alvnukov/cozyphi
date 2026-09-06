---
id: developer-mode-hooks
title: 11 — Показать источники и загруженность hooks без выполнения
status: in_progress
priority: medium
model_level: medium
task_type: feature
parent_id: developer-mode-readonly
tags:
    - developer-mode
acceptance_criteria:
    - integrations.hooks показывает known sources, precedence, configured/loaded и безопасные ошибки.
    - Видны тип события и безопасные свойства применения/ограничений без скрипта, env, команды или sensitive arguments.
    - Collector не выполняет hook, не делает rediscovery/reload и не изменяет hook policy.
    - Обычные pre/post hooks самого harness tool call продолжают работать согласно executor; запрет collector execution не отключает tool loop.
    - Отсутствующий manager и stale loaded config явно отличаются от пустого набора hooks.
verification_plan:
    - Fixture global/project precedence, отсутствующий manager и loaded state после изменения config.
    - Collector-only spies доказывают отсутствие hook execution/reload; executor integration сохраняет обычные pre/post hooks.
    - Sentinel scripts/args/env/errors не попадают в harness result/audit.
created_at: "2026-09-06T09:08:35.745834Z"
updated_at: "2026-09-06T15:03:03.307348Z"
---

## Body

**Что построить.** Наблюдение загруженных hooks и их источников через harness integrations. Читать epic developer-mode-readonly.

**Blocked by:** developer-mode-headless.

**Фиксированные решения.** Snapshot строится по данным manager и безопасным metadata загрузчика. Не перечитывать/выполнять скрипты для определения статуса. Не путать наблюдаемый hook со штатным pre/post hook текущего вызова harness: tool-loop invariant остаётся в силе. Source path sanitization обязательна, raw load errors недопустимы.

**Работа.** Task worktree, scoped tests, closeout по epic.

## Acceptance Criteria

- integrations.hooks показывает known sources, precedence, configured/loaded и безопасные ошибки.
- Видны тип события и безопасные свойства применения/ограничений без скрипта, env, команды или sensitive arguments.
- Collector не выполняет hook, не делает rediscovery/reload и не изменяет hook policy.
- Обычные pre/post hooks самого harness tool call продолжают работать согласно executor; запрет collector execution не отключает tool loop.
- Отсутствующий manager и stale loaded config явно отличаются от пустого набора hooks.

## Verification Plan

1. Fixture global/project precedence, отсутствующий manager и loaded state после изменения config.
2. Collector-only spies доказывают отсутствие hook execution/reload; executor integration сохраняет обычные pre/post hooks.
3. Sentinel scripts/args/env/errors не попадают в harness result/audit.

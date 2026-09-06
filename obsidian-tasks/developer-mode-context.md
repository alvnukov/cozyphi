---
id: developer-mode-context
title: 07 — Показать контекст, compaction и метаданные загрузки
status: done
priority: medium
model_level: medium
task_type: feature
parent_id: developer-mode-readonly
tags:
    - developer-mode
acceptance_criteria:
    - harness context показывает окно, бюджеты, использование, измеренный/оценочный характер данных и состояние compaction.
    - Доступны безопасные metadata источников instructions/skills/memory, scope и загруженность, без текста или previews.
    - Explain различает configured limits, loaded inputs и effective budgets; неизвестное происхождение обозначено явно.
    - Snapshot не инициирует compaction, загрузку/поиск memory, чтение дополнительных skills или изменение калибровки.
    - Снимок ограничен по размеру и detached; существующий context tool сохраняет своё поведение.
verification_plan:
    - 'Fixture загрузки instructions/skills/memory и token observations: verify числа, provenance, estimate/measurement.'
    - Sentinel text в prompt/previews/memory никогда не присутствует в harness ответах/errors/audit.
    - Сравнить состояние до/после чтения; spies compact/load не вызваны, snapshots detached.
created_at: "2026-09-06T09:07:38.768923Z"
updated_at: "2026-09-06T13:17:32.732891Z"
---

## Body

**Что построить.** Завершённая context-категория поверх существующего учёта контекста и загруженных источников. Читать epic developer-mode-readonly.

**Blocked by:** developer-mode-headless.

**Фиксированные решения.** Повторно использовать безопасные количественные измерения владельца; не экспортировать расширенный ContextReport с previews. Не считать оценку токенов точным измерением. Метаданные источников и счётчики должны поступать из уже состоявшейся загрузки, не из повторного чтения prompt/memory. Snapshot — только status, не alias context compact.

**Работа.** Task worktree, scoped tests, closeout по epic.

## Acceptance Criteria

- harness context показывает окно, бюджеты, использование, измеренный/оценочный характер данных и состояние compaction.
- Доступны безопасные metadata источников instructions/skills/memory, scope и загруженность, без текста или previews.
- Explain различает configured limits, loaded inputs и effective budgets; неизвестное происхождение обозначено явно.
- Snapshot не инициирует compaction, загрузку/поиск memory, чтение дополнительных skills или изменение калибровки.
- Снимок ограничен по размеру и detached; существующий context tool сохраняет своё поведение.

## Verification Plan

1. Fixture загрузки instructions/skills/memory и token observations: verify числа, provenance, estimate/measurement.
2. Sentinel text в prompt/previews/memory никогда не присутствует в harness ответах/errors/audit.
3. Сравнить состояние до/после чтения; spies compact/load не вызваны, snapshots detached.

---
id: verify-model-edit-reliability
title: Проверить надёжность model-facing правок
status: todo
priority: high
model_level: medium
task_type: test
parent_id: reliable-model-file-edits
acceptance_criteria:
    - Сквозные тесты покрывают exact edit, anticipated multi-edit shift, read→edit→edit, write→edit, ambiguous hash, mixed grants, overlap, external/concurrent modification и unique/ambiguous plan binding
    - Analyzer считает exact, rebased, refused и recovered по стабильным codes с совместимостью со старыми логами
    - CHANGELOG и документация описывают пользовательское поведение, bounds и fail-closed гарантии
    - Полные fmt-check, lint и test проходят
    - 'Отчёт сравнивает новые сессии с baseline: stale/no-capability -70% и blind retries -80%, либо фиксирует обоснованный недобор без подмены метрик'
verification_plan:
    - Запустить targeted integration и race-тесты всех затронутых пакетов
    - Запустить make fmt-check lint test
    - Сгенерировать свежие сценарные транскрипты и прогнать scripts/analyze_edit_errors.py
    - Сравнить error/recovery показатели с baseline и приложить числа к задаче
    - Проверить документацию и CHANGELOG на соответствие фактическому интерфейсу
created_at: "2026-09-04T22:22:15.439295Z"
updated_at: "2026-09-04T22:22:15.439295Z"
---

## Body

Закрепить поведение всех изменений на model-facing уровне и замкнуть измерительный контур. Эта задача не должна заново реализовывать resolver или capability lifecycle: она добавляет сквозные сценарии, документацию, stable telemetry и сравнение с baseline.

Baseline из 227 транскриптов: edit 500/2905 (17.2%), stale_anchors 264, no_capability 117, plan_gate 75, tag_mismatch 56, blind retries 385. Старые транскрипты не переписывать; analyzer должен различать исторические текстовые классификации и новые stable codes.

Безопасностные сценарии важнее снижения error rate: никакого silent overwrite при внешнем TAG change, неоднозначности, mixed grants, overlap или concurrent write.

**Blocked by:** reanchor-shifted-edit-ranges, chain-edit-successor-capability, authorize-post-write-edits, auto-bind-unique-plan-step.

## Acceptance Criteria

- Сквозные тесты покрывают exact edit, anticipated multi-edit shift, read→edit→edit, write→edit, ambiguous hash, mixed grants, overlap, external/concurrent modification и unique/ambiguous plan binding
- Analyzer считает exact, rebased, refused и recovered по стабильным codes с совместимостью со старыми логами
- CHANGELOG и документация описывают пользовательское поведение, bounds и fail-closed гарантии
- Полные fmt-check, lint и test проходят
- Отчёт сравнивает новые сессии с baseline: stale/no-capability -70% и blind retries -80%, либо фиксирует обоснованный недобор без подмены метрик

## Verification Plan

1. Запустить targeted integration и race-тесты всех затронутых пакетов
2. Запустить make fmt-check lint test
3. Сгенерировать свежие сценарные транскрипты и прогнать scripts/analyze_edit_errors.py
4. Сравнить error/recovery показатели с baseline и приложить числа к задаче
5. Проверить документацию и CHANGELOG на соответствие фактическому интерфейсу

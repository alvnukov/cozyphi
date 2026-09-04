---
id: reliable-model-file-edits
title: Надёжные правки файлов и восстановление модели
status: todo
priority: high
model_level: medium
task_type: epic
acceptance_criteria:
    - Все дочерние задачи завершены и их сквозные сценарии проходят
    - Однозначные механические ошибки якорей и plan binding исправляются автоматически
    - Внешнее изменение файла, неоднозначный re-anchor, mixed grants и overlap остаются fail-closed
    - На новых сессиях stale/no-capability снижаются минимум на 70%, blind retries — минимум на 80% относительно baseline
    - Документация описывает capability lifecycle и гарантии безопасности
verification_plan:
    - Проверить статусы и acceptance criteria всех дочерних задач
    - Запустить полный набор Go-тестов и сценарии надёжности правок
    - Прогнать analyzer по новым сессиям и сравнить показатели с baseline
    - Проверить fail-closed сценарии конкурентного и внешнего изменения файла
created_at: "2026-09-04T22:21:06.938749Z"
updated_at: "2026-09-04T22:21:06.938749Z"
---

## Body

Сделать работу модели с файлами надёжной по принципу correct-by-construction: харнесс автоматически исправляет однозначные ошибки bookkeeping, сохраняет fail-closed при конфликте и даёт ровно один recovery-шаг при отказе.

**Baseline:** анализ 227 транскриптов: edit 500/2905 ошибок (17.2%), stale_anchors 264, no_capability 117, plan_gate 75, tag_mismatch 56, blind retries 385.

**Целевой дизайн:** глубокий модуль жизненного цикла наблюдённых ревизий скрывает provenance grants, typed outcomes, same-TAG re-anchoring и successor capabilities. Read/grep/edit/write остаются тонкими адаптерами. Plan binding исправляется отдельно на seam plan gate.

**Безопасность:** не переносить edit через внешний TAG change; не угадывать при повторяющихся hash, разных delta или разных grants; permission gate не обходить; не допускать silent overwrite.

Дочерние задачи реализуются в порядке их явных blocking edges. Epic не закрывать до сквозной проверки и сравнения с baseline.

## Acceptance Criteria

- Все дочерние задачи завершены и их сквозные сценарии проходят
- Однозначные механические ошибки якорей и plan binding исправляются автоматически
- Внешнее изменение файла, неоднозначный re-anchor, mixed grants и overlap остаются fail-closed
- На новых сессиях stale/no-capability снижаются минимум на 70%, blind retries — минимум на 80% относительно baseline
- Документация описывает capability lifecycle и гарантии безопасности

## Verification Plan

1. Проверить статусы и acceptance criteria всех дочерних задач
2. Запустить полный набор Go-тестов и сценарии надёжности правок
3. Прогнать analyzer по новым сессиям и сравнить показатели с baseline
4. Проверить fail-closed сценарии конкурентного и внешнего изменения файла

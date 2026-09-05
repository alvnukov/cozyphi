---
id: reanchor-shifted-edit-ranges
title: Безопасно исправлять сдвинутые edit-якоря
status: done
priority: high
model_level: medium
task_type: feature
parent_id: reliable-model-file-edits
branch: feature/reanchor-shifted-edit-ranges
worktree_path: .worktrees/reanchor-shifted-edit-ranges
acceptance_criteria:
    - Сдвинутые from/to автоматически сопоставляются с наблюдённым диапазоном только при том же path, full-file TAG и одном grant
    - Автокоррекция применяется только для уникальной пары кандидатов с одинаковым line delta и непересекающимися диапазонами
    - Повторяющиеся короткие hash, разные delta, mixed grants, ненаблюдённые якоря и изменённый TAG отклоняются типизированно
    - Результат edit сообщает, что координаты исправлены, и какой delta применён, не раскрывая содержимое правок
    - Обычные exact-anchor edits сохраняют прежнее поведение
verification_plan:
    - 'Добавить table-driven resolver-тесты: exact, positive/negative delta, multiple ranges, duplicate hash, mixed grants, mismatched delta'
    - Добавить интеграционный тест multi-edit с anticipated shift, проверяющий итоговый файл
    - Запустить go test для editledger и writetool
    - Запустить race-тест затронутых пакетов
created_at: "2026-09-04T22:21:27.035557Z"
updated_at: "2026-09-05T00:19:27.681675Z"
---

## Body

Научить edit resolver воспринимать номер строки как подсказку, а принадлежность hash к наблюдённому grant — как provenance. Типовая ошибка модели в multi-edit: второй диапазон получает номера после предполагаемого сдвига, хотя все диапазоны относятся к исходному snapshot. Харнесс должен механически исправлять только однозначный одинаковый сдвиг.

Resolver работает исключительно внутри сохранённой наблюдённой ревизии; короткий hash не используется для произвольного поиска по файлу. Если hash повторяется и точный диапазон определить нельзя, edit остаётся fail-closed.

**Blocked by:** review-model-edit-reliability-design, typed-edit-capability-outcomes — сначала утверждаются safety matrix и typed resolver outcome.

**Done (2026-09-05).** Edit anchors that are uniformly shifted now re-anchor instead of refusing. editledger.Claim resolves typed refs against grant line-hashes: unique per-hash match inside one grant, single shared delta, non-overlapping ranges ⇒ Claim granted with Resolution{Outcome: Rebased, Delta, Lines}; anything ambiguous ⇒ typed refusal ambiguous_reanchor, file untouched. writetool reports `rebased edits[i] from X-Y to Z-W (delta ±N)` in the result body. Exact anchors unchanged. doc/edit-capability.md Today-row refreshed; CHANGELOG [Unreleased]. Landed: 75feca2 (fix(tools): expect typed refusal code in edit authorization test — fallout of task 2/7, failed on clean HEAD too) + 545aae1 (feat) merged as 30cf4a8, worktree and branch removed. Gates: golangci-lint 0, go test ./internal/tools/... all ok, -race editledger+writetool ok.

## Acceptance Criteria

- Сдвинутые from/to автоматически сопоставляются с наблюдённым диапазоном только при том же path, full-file TAG и одном grant
- Автокоррекция применяется только для уникальной пары кандидатов с одинаковым line delta и непересекающимися диапазонами
- Повторяющиеся короткие hash, разные delta, mixed grants, ненаблюдённые якоря и изменённый TAG отклоняются типизированно
- Результат edit сообщает, что координаты исправлены, и какой delta применён, не раскрывая содержимое правок
- Обычные exact-anchor edits сохраняют прежнее поведение

## Verification Plan

1. Добавить table-driven resolver-тесты: exact, positive/negative delta, multiple ranges, duplicate hash, mixed grants, mismatched delta
2. Добавить интеграционный тест multi-edit с anticipated shift, проверяющий итоговый файл
3. Запустить go test для editledger и writetool
4. Запустить race-тест затронутых пакетов

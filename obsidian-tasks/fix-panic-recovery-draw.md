---
id: fix-panic-recovery-draw
title: 'нет recover в draw-пути: паника на середине кадра убивает процесс'
status: done
priority: medium
task_type: bug
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - паника в Draw не убивает приложение, показывает ошибку
verification_plan:
    - тест с паникующим виджетом
created_at: "2026-08-23T15:17:22.115362Z"
updated_at: "2026-09-02T19:01:40.130251Z"
---

## Body

internal/components/app/app.go:271-303 — вокруг отрисовки ~15-виджетного дерева нет recover. Паника поверхностной математики убивает процесс mid-frame; deferred vx.Close() восстановит терминал, но хуки/пулы текут как в fix-ctrl-c-skips-close. Фикс: recover в paint + сообщение об ошибке вместо падения.

## Acceptance Criteria

- паника в Draw не убивает приложение, показывает ошибку

## Verification Plan

1. тест с паникующим виджетом

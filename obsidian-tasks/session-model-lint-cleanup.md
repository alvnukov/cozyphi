---
id: session-model-lint-cleanup
title: Clean up existing session and model lint findings
status: todo
priority: low
model_level: low
task_type: chore
tags:
    - lint
acceptance_criteria:
    - Устранить четыре замечания perfsprint/usetesting без изменения поведения после проверки актуального main.
verification_plan:
    - Проверить актуальность четырёх замечаний в main.
    - Исправить в отдельном worktree; запустить scoped tests/lint.
created_at: "2026-09-06T11:27:16.527199Z"
updated_at: "2026-09-06T11:27:16.527199Z"
---

## Body

Единственный scoped lint задачи multisession-title-tool на базе d3ca734 сообщил четыре замечания в неизменённых файлах: internal/agent/model_selection.go:32 fmt.Errorf без форматирования (perfsprint); internal/tui/sessions/lifecycle_ownership_test.go:104,182,187 context.Background вместо t.Context (usetesting). git diff d3ca734 по этим файлам пуст. Лог: /tmp/cozyphi-title-tool-lint.log. Не чинить попутно с именованием; проверить актуальность отдельно.

## Acceptance Criteria

- Устранить четыре замечания perfsprint/usetesting без изменения поведения после проверки актуального main.

## Verification Plan

1. Проверить актуальность четырёх замечаний в main.
2. Исправить в отдельном worktree; запустить scoped tests/lint.

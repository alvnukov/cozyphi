---
id: xui-input-lint-debt
title: Устранить существующие замечания lint в xui/input
status: todo
priority: low
model_level: medium
task_type: chore
tags:
    - lint
    - xui
acceptance_criteria:
    - golangci-lint run ./input в модуле xui проходит.
verification_plan:
    - cd xui && go test ./input
    - cd xui && golangci-lint run ./input
created_at: "2026-09-05T22:08:18.473174Z"
updated_at: "2026-09-05T22:08:18.473174Z"
---

## Body

При проверке fix-long-prompt-input обнаружены существующие ошибки lint в xui/input: parser.go G115 rune(alt), rune(codepoint); ineffassign press = true; modernize min; layout_test.go misspell behaviour. Строки не изменены исправлением вставки, присутствуют в базе 99b3380. Исправить отдельно с проверкой безопасности преобразований.

## Acceptance Criteria

- golangci-lint run ./input в модуле xui проходит.

## Verification Plan

1. cd xui && go test ./input
2. cd xui && golangci-lint run ./input

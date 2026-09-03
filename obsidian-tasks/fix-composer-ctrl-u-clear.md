---
id: fix-composer-ctrl-u-clear
title: Ctrl+U не очищает композер
status: done
priority: low
task_type: bug
parent_id: fix-composer-alt-enter-newline
tags:
    - tui
    - composer
    - keys
    - review-2026-09
acceptance_criteria:
    - Ctrl+U в композере делает задокументированное действие
    - Тест на клавишу
verification_plan:
    - go test ./internal/components/chat/
created_at: "2026-09-02T16:51:43.324808Z"
updated_at: "2026-09-02T20:41:48.860369Z"
---

## Body

**Проблема.** Тело задачи fix-composer-alt-enter-newline называло второй симптом «C-u не очищает композер»; он вне её критериев и не тронут: в `internal/components/chat/chat_input.go` обработки Ctrl+U нет.

**Как чинить.** Решить, какая семантика нужна (очистить строку до курсора, как в readline, или весь композер), добавить обработку и тест. Найдено ревью правок после v0.19.0.

## Acceptance Criteria

- Ctrl+U в композере делает задокументированное действие
- Тест на клавишу

## Verification Plan

1. go test ./internal/components/chat/

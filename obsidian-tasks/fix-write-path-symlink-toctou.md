---
id: fix-write-path-symlink-toctou
title: 'write-путь: закрыть symlink TOCTOU между permission gate и записью'
status: todo
priority: high
task_type: bug
parent_id: permission-symlink-workspace-escape
tags:
    - security
    - permissions
    - symlink
    - toctou
acceptance_criteria:
    - Атакующий не может подменить проверенный path/symlink между gate и mutation.
    - write и edit используют один безопасный filesystem mutation module.
    - Ошибки fail closed и называют проверяемый путь без утечки содержимого.
verification_plan:
    - Детерминированный race fixture меняет symlink между проверкой и open; внешний файл остаётся неизменным.
created_at: "2026-08-23T15:17:22.114064Z"
updated_at: "2026-08-24T13:20:52.029063Z"
---

## Body

os.WriteFile следует symlink после отдельной lexical permission-проверки. Даже EvalSymlinks в gate оставляет окно подмены. Реализовать descriptor-relative/openat-style safe write или эквивалентную платформенную стратегию и переиспользовать её в write/edit.

## Acceptance Criteria

- Атакующий не может подменить проверенный path/symlink между gate и mutation.
- write и edit используют один безопасный filesystem mutation module.
- Ошибки fail closed и называют проверяемый путь без утечки содержимого.

## Verification Plan

1. Детерминированный race fixture меняет symlink между проверкой и open; внешний файл остаётся неизменным.

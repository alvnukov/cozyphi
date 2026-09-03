---
id: cozy-tools-extraction
title: 'Этап 2: тулы из mcp-ai-helper → библиотека cozy-tools'
status: todo
priority: low
task_type: feature
parent_id: cozyphi-convenience-program
tags:
    - tools
    - mcp
    - phase-2
acceptance_criteria:
    - cozy-tools как отдельный модуль с тестами
    - cozyphi использует тулы из cozy-tools
    - permission gate распространяется на новые тулы
verification_plan:
    - make test в обоих репозиториях
    - проверка набора тулов в сессии
created_at: "2026-08-23T14:45:57.938706Z"
updated_at: "2026-08-23T14:45:57.938706Z"
---

## Body

Выделить тулы mcp-ai-helper (file/edit/command/git/task и др.) в отдельную Go-библиотеку cozy-tools и встроить их в cozyphi вместо минималистичного набора phi. Этап 2 после удобства UI.

## Acceptance Criteria

- cozy-tools как отдельный модуль с тестами
- cozyphi использует тулы из cozy-tools
- permission gate распространяется на новые тулы

## Verification Plan

1. make test в обоих репозиториях
2. проверка набора тулов в сессии

---
id: refactor-tool-owned-permissions
title: 'пермишены: extraction за интерфейсом Tool вместо строкового switch'
status: todo
priority: medium
task_type: refactor
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - новый тул декларирует свой гейтинг сам
    - extract.go без default-ветки по именам
verification_plan:
    - go test ./internal/permission/...
created_at: "2026-08-23T15:17:22.117539Z"
updated_at: "2026-08-23T15:17:22.117539Z"
---

## Body

permission/extract.go:107-109 — switch по имени тула; каждое новое имя требует правки центрального файла; mcp_call падает в unknown. tooldef.Tool уже владеет именем/схемой/DetailFromArgs — добавить опциональный дескриптор 'что трогает этот тул' (bash command, file path, agent action). Поглощает fix-mcp-call-permission-default; знание живёт рядом с парсером каждого тула.

## Acceptance Criteria

- новый тул декларирует свой гейтинг сам
- extract.go без default-ветки по именам

## Verification Plan

1. go test ./internal/permission/...

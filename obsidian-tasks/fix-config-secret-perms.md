---
id: fix-config-secret-perms
title: 'секреты: config.yaml 0644, /api/config отдаёт ключи, сессии 0644'
status: done
priority: medium
task_type: bug
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - config.yaml/.bak/сессии 0600
    - API отдаёт маскированные ключи
verification_plan:
    - ls -l после phi config; curl /api/config
created_at: "2026-08-23T15:17:22.113368Z"
updated_at: "2026-08-23T19:28:29.832074Z"
---

## Body

cmd/config.go:452 пишет config.yaml (plaintext API-ключи) и .bak с 0644; GET /api/config (:165-174) возвращает ключи plaintext JSON (loopback, но любой локальный процесс); session JSONL тоже 0644. Ключи не логируются и не попадают в транскрипт (ок), hooks/sanitize.go чистит env (ок). Фикс: 0600 на конфиг/сессии, /api/config маскирует ключи.

## Acceptance Criteria

- config.yaml/.bak/сессии 0600
- API отдаёт маскированные ключи

## Verification Plan

1. ls -l после phi config; curl /api/config

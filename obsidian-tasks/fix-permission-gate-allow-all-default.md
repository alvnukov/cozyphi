---
id: fix-permission-gate-allow-all-default
title: 'Гейт пермишенов: allowAll=true по умолчанию'
status: done
priority: high
task_type: bug
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - дефолт — спрашивать
    - --yolo/dangerously_allow_all отключает
    - конфиг не может молча включить allowAll
verification_plan:
    - ручная проверка промпта на bash-туле
created_at: "2026-08-23T14:45:57.934908Z"
updated_at: "2026-08-23T19:17:16.248363Z"
---

## Body

Конструктор контроллера c.allowAll.Store(true) (internal/tui/controller/controller.go:88) — тулы исполняются без промптов; initGate (:165-186) явно отказывается вернуть запросы, когда конфиг omits dangerously_allow_all. Должно быть: по умолчанию спрашивает, флаг отключает (upstream уже завезли --yolo, 508f2fa). Зеркалит ось Security в AGENTS.md.

## Acceptance Criteria

- дефолт — спрашивать
- --yolo/dangerously_allow_all отключает
- конфиг не может молча включить allowAll

## Verification Plan

1. ручная проверка промпта на bash-туле

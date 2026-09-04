---
id: refactor-run-error-hints-reuse-classifier
title: phi run дублирует подсказки классификатора ошибок
status: done
priority: medium
task_type: refactor
parent_id: fix-stream-error-stringly
tags:
    - cli
    - errors
    - design
    - review-2026-09
acceptance_criteria:
    - Тексты подсказок cancel/auth/rate-limit существуют в одном месте
    - Одно имя classifyRunError с одной семантикой
verification_plan:
    - go test ./cmd/ ./internal/tui/controller/
created_at: "2026-09-02T16:51:43.323395Z"
updated_at: "2026-09-04T17:50:24.067181Z"
---

## Body

**Проблема.** `cmd/run.go:211-219` завёл свой switch по `IsCanceled/IsAuthFailure/IsRateLimited` с копиями подсказок из `internal/tui/controller/runerror.go:38-47`. `internal/tui/DESIGN.md` §Notices требует переиспользовать классификатор, а не печатать свои тексты. Две функции `classifyRunError` с разной семантикой: `cmd/run.go:523` возвращает код выхода, `controller/runerror.go:33` сообщение.

**Мелочь рядом.** `IsRateLimited` хардкодит 529 без именованной константы (Anthropic overloaded).

**Как чинить.** Вынести классификацию в пакет, доступный и TUI, и `phi run`: одна функция даёт причину и подсказку, cmd маппит её на код выхода; одно из имён classifyRunError переименовать. Найдено ревью правок после v0.19.0.

**Note (2026-09-04).** 2026-09-04, аудит имён: в тексте задачи везде читай `cozyphi run` вместо phi run — бинарник называется cozyphi.

## Acceptance Criteria

- Тексты подсказок cancel/auth/rate-limit существуют в одном месте
- Одно имя classifyRunError с одной семантикой

## Verification Plan

1. go test ./cmd/ ./internal/tui/controller/

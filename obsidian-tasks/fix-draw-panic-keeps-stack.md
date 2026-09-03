---
id: fix-draw-panic-keeps-stack
title: восстановленная паника в Draw теряет стек
status: done
priority: low
task_type: bug
parent_id: fix-panic-recovery-draw
tags:
    - tui
    - reliability
    - review-2026-09
acceptance_criteria:
    - Стек паники попадает в debuglog
    - Сообщение на экране называет, где искать стек
verification_plan:
    - go test ./internal/components/app/
created_at: "2026-09-02T16:51:43.324495Z"
updated_at: "2026-09-02T20:14:33.836213Z"
---

## Body

**Проблема.** `internal/components/app/app.go:330-337` превращает панику кадра в одну обрезанную по ширине строку, которую следующее событие перерисовывает; стек выбрасывается, разбирать причину нечем. Заодно `errorSurface` (`app.go:353`) вручную раскладывает руны с `Width: 1`, хотя `components.PaintSpans` умеет мерить ширину.

**Как чинить.** Логировать `debug.Stack()` через `debuglog` при recover, на экране называть, где искать стек; рисовать через PaintSpans. Найдено ревью правок после v0.19.0.

## Acceptance Criteria

- Стек паники попадает в debuglog
- Сообщение на экране называет, где искать стек

## Verification Plan

1. go test ./internal/components/app/

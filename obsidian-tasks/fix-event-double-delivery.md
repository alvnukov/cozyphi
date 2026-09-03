---
id: fix-event-double-delivery
title: непотреблённые события доставляются виджету дважды
status: done
priority: medium
task_type: bug
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - событие доходит до виджета один раз
    - тест double-delivery
verification_plan:
    - go test ./internal/components/app/... ./internal/tui/editor/...
created_at: "2026-08-23T15:17:22.114717Z"
updated_at: "2026-09-02T19:01:40.126389Z"
---

## Body

internal/components/app/app.go:231-242 dispatch шлёт focused, затем root; editor.go:297-299 Editor.Handle форвардит всё в ComposerPane.Handle; pane.go:477 и 450-455 повторно зовут Chat.Handle и palette.Handle — те же виджеты, только что отказавшиеся от клавиши. Сегодня безвредно только потому, что каждая мутирующая ветка делает ConsumeAndRedraw; будущая неконсьюмящая мутация задвоится.

## Acceptance Criteria

- событие доходит до виджета один раз
- тест double-delivery

## Verification Plan

1. go test ./internal/components/app/... ./internal/tui/editor/...

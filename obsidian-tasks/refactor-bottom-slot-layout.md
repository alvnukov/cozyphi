---
id: refactor-bottom-slot-layout
title: 'Layout: bottom-slot module для арбитража оверлей/композер и Z-лестницы'
status: done
priority: medium
task_type: refactor
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - арифметика высот в одном module
    - Z-лестница именована
    - listH-перезапись устранена или объяснена
verification_plan:
    - go test ./internal/components/...
created_at: "2026-08-23T15:17:22.121379Z"
updated_at: "2026-08-24T13:49:44.003127Z"
---

## Body

Editor.Draw (editor.go:320-386) — ручная арифметика: min/max высоты чата дублированы (editor.go:335-347 и pane.go:309-316 идентичны), Z-константы рассеяны (1,2 / 15 / 20 / 40 / 50), editor.go:356-357 молча перетирает только что вычисленный listH последней высотой транскрипта. Кандидат: widget-стек в components, спрашивающий у кандидата PreferredHeight и владеющий Z-лестницей; interface — 'дай свою surface для этого слота'. Арифметика получает один дом.

## Acceptance Criteria

- арифметика высот в одном module
- Z-лестница именована
- listH-перезапись устранена или объяснена

## Verification Plan

1. go test ./internal/components/...

---
id: refactor-hover-affordance-followups
title: 'hover-подсветка: дубли в шести блоках, двойной HitTest, нет описания в DESIGN.md'
status: done
priority: low
task_type: refactor
parent_id: fix-hover-affordance-visual
tags:
    - tui
    - hover
    - review-2026-09
acceptance_criteria:
    - Подсветка заголовка вызывается через один хелпер
    - Один HitTestAt на событие мыши
    - DESIGN.md описывает hover-подсветку
verification_plan:
    - go test ./internal/components/... ./internal/tui/...
created_at: "2026-09-02T16:51:43.324184Z"
updated_at: "2026-09-02T20:03:48.652063Z"
---

## Body

**Дубли.** Одинаковый трёхстрочный блок `if components.Hovering(ctx, x) && x.HasBody() { components.ApplyHoverRows(&s, 0, x.titleH, th.BackgroundElement) }` лежит в шести файлах `internal/components/block` (например `tool_block.go:198`); хелпер `hoverTitle(ctx, &s, w, titleH)` в пакете block избавит следующий блок от забывчивости.

**Двойной HitTest.** `updateHover` (`internal/components/app/pointer.go:33`) и `handleEvent` (`app.go:201`) оба гоняют `lastSurf.HitTestAt` на каждое событие мыши; `pointerShapeAt` живёт только ради тестов.

**Док.** `internal/tui/DESIGN.md` называет себя контрактом ленты, но hover-подсветку не описывает. Найдено ревью правок после v0.19.0.

## Acceptance Criteria

- Подсветка заголовка вызывается через один хелпер
- Один HitTestAt на событие мыши
- DESIGN.md описывает hover-подсветку

## Verification Plan

1. go test ./internal/components/... ./internal/tui/...

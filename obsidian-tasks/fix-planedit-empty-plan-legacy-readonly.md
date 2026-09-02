---
id: fix-planedit-empty-plan-legacy-readonly
title: 'Planedit: absent plan read as legacy — first plan cannot be created from the editor'
status: done
task_type: bug
tags:
    - tui
    - plan
    - planedit
    - phase1
verification_plan:
    - go test ./internal/tui/planedit/... ./internal/tui/editor/... ./internal/session/...
    - make fmt-check lint test
    - 'ручная проверка: пустая сессия → /plan → редактирование и сохранение создаёт v2-план; легаси-сессия → readonly'
created_at: "2026-09-01T14:30:33.031308Z"
updated_at: "2026-09-01T15:00:14.812583Z"
---

## Body

**Symptom.** Сессия без плана: открыть редактор плана → черновик выглядит пустым, но любое изменение/сохранение даёт «legacy plan: only v2 plans can be edited».

**Cause.** planedit Pane.Show() (internal/tui/planedit/pane.go ~822) помечает readonly любой снапшот с !Schema.IsV2(). Нулевой session.Plan{} (плана нет вообще, Schema 0, Revision 0) спутан с легаси. Второй слой: Store-шов редактора умеет только Apply-патч, а Manager.PatchPlan требует существующий v2-план (plan_patch.go:188) — создать первый план сессии из редактора нечем.

**Fix.** 1) Show(): Revision 0 + Schema 0 → редактируемый пустой черновик; PlanSchemaLegacy → readonly как раньше. 2) apply() при пустом базисе отправляет create-контракт (goal/approach/successCriteria/constraints/steps) через новый путь Store, а не патч.

**Blast radius.** internal/tui/planedit (pane, store seam), internal/tui/editor/planstore.go, возможно controller/engine. Легаси-редонли и v2-редактирование не меняются.

## Verification Plan

1. go test ./internal/tui/planedit/... ./internal/tui/editor/... ./internal/session/...
2. make fmt-check lint test
3. ручная проверка: пустая сессия → /plan → редактирование и сохранение создаёт v2-план; легаси-сессия → readonly

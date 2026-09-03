---
id: plan-gated-tools
title: 'Привязка тулов главного движка к активному пункту утверждённого плана'
status: done
---

# plan-gated-tools

Все тулы главного движка привязываются к активному пункту утверждённого плана.
Справа в панели плана — чекбокс «утвержден». Пока не утверждено, гейт не
вмешивается; после утверждения модель обязана указать `plan_step` в вызове
тула. Пропуск/несуществующий/неактивный шаг на первой фазе не блокирует, а
возвращает модели подсказку и пишет промах в отдельный JSONL-лог; после
анализа статистики фаза переключается на запрет.

## Дизайн

- `internal/plangate` — глубокий модуль: `Check(plan, tool, plan_step)` →
  (pass | hint | deny, miss), фаза `Hint`/`Deny`, тип шага → разрешённые тулы.
- `session.StepType` (explore/edit/run/delegate/integrate) + `Plan.Approved`
  + `PlanItem.Type`; `Manager.ApprovePlan` (append-only PlanEntry).
- executor вызывает plangate между PreTool и permission-gate; hint уходит
  только в model-content, не в TUI.
- prompt получает контракт + таблицу тип→тулы из `plangate.PromptBlock()`.
- лог промахов: `~/.cozyphi/logs/plan-gate-misses.jsonl`.
resolved_by: cfff415 feat(plangate): gate tools by approved plan steps

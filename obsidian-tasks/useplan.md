---
status: done
---

# useplan

Третий режим оборота модели — UsePlan. Чекбокс утверждённого плана становится
ASCII `[ ]`/`[x]` и переключается по Ctrl+A. Модель полностью лишается тулов,
если в вызове нет `plan_step` пункта плана в работе; Build тулы не блокирует.
Режим окрашивается в фиалковый (Violet) оттенок.

## Дизайн

- `agent.ModeUsePlan` + `normalizeMode`; фаза plan-gate по режиму: UsePlan →
  `Deny`, Build/Plan → `Hint`.
- `InjectPlanStep` применяется к итоговому `buildToolList`, чтобы `agent_*` и
  `mcp_*` тоже получили `plan_step`.
- Валиден только `PlanInProgress` шаг; `PromptBlock` говорит «in_progress».
- `Controller.ToggleMode` трёхстадийный: build → plan → useplan → build;
  UsePlan не накладывает readonly-overlay.
- `Theme.Violet` во всех 6 палитрах; composer/editor три лейбла и цвета.
- Unapprove во время стрима: отмена стрима + сброс очереди; approve во время
  стрима отклоняется.
- Sidebar: `HandleApproveKey` (Ctrl+A), тосты «План одобрен» / «План остановлен».
resolved_by: 05f1004 feat(useplan): gate model tools by in-progress plan step

---
id: useplan-default-mode
title: 'UsePlan как режим оборота модели по умолчанию'
status: done
---

# useplan-default-mode

Режим оборота модели по умолчанию — UsePlan. При старте движок и контроллер
переходят в useplan, plan-gate сразу в фазе Deny. Пустой/неизвестный режим
нормализуется в useplan; build остаётся явным режимом. Композер по умолчанию
показывает фиалковый лейбл `⏵⏵ useplan`.

## Дизайн

- `agent.normalizeMode` → `ModeUsePlan` для пустых/неизвестных значений;
  `ModeBuild` остаётся явным кейсом.
- `agent.Engine` стартует с `mode = ModeUsePlan`; `applyPlanGatePhase()` при
  старте переводит plan-gate в `PhaseDeny`.
- `controller.Controller`: `NewController` стартует с `ModeUsePlan`, `Mode()` /
  `SetMode` / `ToggleMode` согласованы; цикл тоггла useplan → build → plan →
  useplan.
- `composer` по умолчанию: `⏵⏵ useplan` / Violet.
- Изоляция теста spawn-валидации от plan-gate через `SetMode(ModeBuild)`.
resolved_by: f910705 feat(agent): make useplan the default turn posture

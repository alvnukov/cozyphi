---
status: done
---

# fix-plan-approval-handoff

Ctrl+A «утвердить» не передавал управление модели: пока идёт стрим, одобрение
отклонялось (`cannot approve the plan while a reply is running`), а завершённый
turn, заблокированный на неутверждённом плане, никто не возобновлял.

## Дизайн

- `plangate.ReasonPlanNotApproved` — экспортированная строка deny-причины;
  `plangate.IsExempt` — является ли тул «plan/context».
- `Controller.planGateBlocked` — флаг «последний run заблокирован на
  неутверждённом плане»; ставится на `ToolRejected` с этой причиной, снимается
  на терминальном `ToolDone/Error/Rejected/Cancelled` не-exempt тула, сбрасывается
  в `startPromptLocked`.
- `SetPlanApproved(true)` больше не отклоняется mid-run; в ин-флайт цикле модель
  перепроверит гейт на следующем tool call. Если стрим уже завершён и флаг
  заблокирован — запускается resume-промпт.
- `finishRun` тоже вызывает `maybeResumeBlockedLocked`: одобрение во время
  стрима, после которого модель закончила turn, возобновляется без ручного
  re-prompt.
- Editor: при ошибке одобрения не показывается ложный toast «План остановлен».
resolved_by: 211a255 fix(useplan): approve hands control back to a blocked turn

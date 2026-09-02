---
id: fix-plan-skill-inject-delivery
title: inject_skill из плана доезжает следующим промптом — модель получает скиллы после закрытия шага
status: done
priority: high
task_type: bug
tags:
    - plan
    - skills
    - agent
    - prompt
    - injection
acceptance_criteria:
    - 'Тест-двойник TestPlanActionAdviceRidesItsOwnToolResult для скиллов: во втором запросе инструкция видна внутри результата того самого вызова ровно один раз'
    - Следующий промпт не повторяет инструкцию; off-метки по-прежнему исключают скилл (TestInjectSkillQueuesOnlyEffectiveSkills обновлён)
    - make fmt-check lint test зелёные; фикс смержен в main
verification_plan:
    - go test ./internal/agent/ -run 'InjectSkill|PlanActionAdvice' — новые и обновлённые тесты зелёные
    - make fmt-check lint test
created_at: "2026-08-31T07:46:58.808587Z"
updated_at: "2026-08-31T08:20:51.990027Z"
---

## Body

**Problem:** план-экшн inject_skill паркует имена скиллов в engine.planSkills, а доезжают они до модели только следующим скомпонованным user-промптом (composeUserPrompt → mergePlanSkills, internal/agent/engine.go:949). Шаг стартует mid-turn — plan_step на tool-вызове или _plan-settle — и обычно закрывается в том же turn: инструкция приходит следующим сообщением, когда работа уже сделана. Наблюдается как «модель игнорирует инжект скиллов из плана».

**Contrast:** пользовательский путь работает — скиллы из композера (StartPrompt(text, skills)) ведут то самое сообщение, которым открывается turn; оба пути рендерит один pendingSkillsInstruction (engine.go:983).

**Root cause:** доставка на одну границу позже. У compact-совета была та же бага и решена in-call дрейном в результат вызова: Executor.SetCompactAdviceDrain (internal/agent/executor.go:74, 386–390; wiring engine.go:514), закреплено тестом TestPlanActionAdviceRidesItsOwnToolResult (engine_plan_actions_test.go:141).

**Fix:** зеркалировать этот seam для скиллов — Executor.SetPlanSkillDrain, дрейн в сборке результата того вызова, которым стартовал шаг (рядом с drainCompactAdvice); рендер тем же pendingSkillsInstruction; очередь остаётся одна (drain-to-empty), так что «ровно один раз» сохраняется; границы без tool-вызова (TUI-approval, sidebar-start) оставляют текущее поведение следующего промпта.

**Scope:** internal/agent/executor.go, internal/agent/engine.go, internal/agent/engine_plan_actions.go, тесты engine_plan_actions_test.go.

## Acceptance Criteria

- Тест-двойник TestPlanActionAdviceRidesItsOwnToolResult для скиллов: во втором запросе инструкция видна внутри результата того самого вызова ровно один раз
- Следующий промпт не повторяет инструкцию; off-метки по-прежнему исключают скилл (TestInjectSkillQueuesOnlyEffectiveSkills обновлён)
- make fmt-check lint test зелёные; фикс смержен в main

## Verification Plan

1. go test ./internal/agent/ -run 'InjectSkill|PlanActionAdvice' — новые и обновлённые тесты зелёные
2. make fmt-check lint test

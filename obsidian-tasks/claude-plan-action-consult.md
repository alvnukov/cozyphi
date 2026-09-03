---
id: claude-plan-action-consult
title: 'Plan action consult_claude: автоматическое ревью на step_end/plan_start'
status: todo
priority: low
model_level: high
task_type: feature
parent_id: claude-consult-tool
tags:
    - claude
    - plan
    - actions
acceptance_criteria:
    - consult_claude выполняется на step_end/plan_start только в approved-плане; advisory-падение не блокирует переход, required — блокирует.
    - Findings доходят до модели как reminder и записаны как PlanActionRun; действие видно и редактируемо в plan editor.
    - make fmt-check lint test rc=0.
verification_plan:
    - go test ./internal/agent/... ./internal/tools/plantool/... -run 'ConsultClaude|PlanAction' -race.
    - make fmt-check lint test.
created_at: "2026-09-02T05:13:31.876583Z"
updated_at: "2026-09-02T05:13:31.876583Z"
---

## Body

**Цель.** Когда ручные режимы докажут ценность — автоматика по образцу specs/plan-actions-and-step-models.md.

**Что сделать.** Встроенный тип действия consult_claude {event: step_end|plan_start, params: {mode: review_diff|review_plan, question?, required: false}}: выполняется синхронно через тот же ClaudeRunner/бриф с автопривязкой вложений (diff шага / проекция плана), результат — PlanActionRun + строка PlanActionRan + reminder модели с findings (не user message). required=false (дефолт): консультация advisory — падение (нет бинаря, таймаут) записывается как failed run, но переход не отклоняется; required=true — по общему правилу spec, отклоняет переход. Действие — пользовательская конфигурация (не model-owned, как inject_skill): проходит approval, редактируется в plan editor. Лимиты economy применяются.

**Тесты.** по образцу engine_plan_actions_test: advisory/required, повтор на reopen, только в approved-плане, reminder содержит findings, run записан.

**Зависит от:** claude-attachments-plan-and-diff, claude-economy-threads-brakes-ledger. Фаза 2.

## Acceptance Criteria

- consult_claude выполняется на step_end/plan_start только в approved-плане; advisory-падение не блокирует переход, required — блокирует.
- Findings доходят до модели как reminder и записаны как PlanActionRun; действие видно и редактируемо в plan editor.
- make fmt-check lint test rc=0.

## Verification Plan

1. go test ./internal/agent/... ./internal/tools/plantool/... -run 'ConsultClaude|PlanAction' -race.
2. make fmt-check lint test.

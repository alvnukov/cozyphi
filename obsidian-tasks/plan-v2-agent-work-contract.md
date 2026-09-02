---
id: plan-v2-agent-work-contract
title: 'Plan v2: качественный контекст, атомарное редактирование и проверяемое выполнение'
status: blocked
priority: high
model_level: very_high
task_type: epic
tags:
    - plan-v2
    - agent
    - context
    - plangate
    - ux
    - tdd
acceptance_criteria:
    - Все implementation и integration child tasks эпика имеют status done и зелёные declared gates.
    - Durable plan v2 сохраняет цель, подход, критерии успеха, ограничения и bounded рабочий контекст без потери после resume/compaction.
    - Шаги имеют stable IDs, action, why и done_when; статусы меняются только валидными переходами, а outcome подтверждается evidence.
    - Обычный цикл не требует отдельного plan call для старта шага и поддерживает переход к следующему рабочему tool call без промежуточного model round.
    - Контрактные изменения требуют reapproval, операционные updates его не сбрасывают; JIT-gated внешние действия требуют отдельного разрешения.
    - Prompt projection укладывается в зафиксированный budget, не содержит полных старых логов и не интерпретирует model-authored plan data как system instructions.
    - Завершённый plan автоматически архивируется и перестаёт постоянно занимать model context.
    - plan-v2-strong-model-review имеет status done после повторного полного review и в эпике нет unresolved review findings.
verification_plan:
    - Проверить status и acceptance evidence всех child/follow-up tasks через task registry.
    - Запустить focused package tests, integration/e2e plan tests и race tests для session, plan tool, executor/plangate и TUI seams.
    - Измерить happy-path API rounds и размер prompt projection на коротком и максимальном плане.
    - Выполнить финальный Standards+Spec review моделью very_high; при findings повторять fix+review до нулевого результата.
created_at: "2026-08-28T10:51:18.603152Z"
updated_at: "2026-08-28T10:51:18.603152Z"
---

## Body

Цель эпика — превратить durable plan из заменяемого списка шагов в компактный, устойчивый к компакции контракт между человеком, моделью и harness. Итоговая система разделяет контракт, рабочий контекст, состояние выполнения и аудит; сохраняет токены; уменьшает число model/tool API rounds; не ослабляет plan gate.

Обязательные решения: plan-level goal, approach, success criteria, constraints и bounded working context; stable step IDs; обязательные action/why/done_when; outcome отдельно от evidence; domain-specific atomic mutations вместо полного snapshot replace; статусы только через state-machine transitions; auto-start pending step при валидном tool call; tool attempts и evidence refs; optional piggyback transition; field-aware approval; compact prompt projection; auto-completion/archive; JIT approval для необратимых внешних действий; двухуровневый sidebar; safety, telemetry, migration compatibility.

Контракт выполнения для всех implementation children: model_level low; один узкий public-seam tracer bullet; сначала наблюдаемый red test, затем минимальный green; без попутного рефакторинга; закрытие через один mcp-ai-helper workflow с focused gate, task transition и owned-files commit.

Условие закрытия эпика: ВСЕ child tasks и все follow-up tasks завершены; затем задача plan-v2-strong-model-review выполнена моделью уровня very_high как полное Standards+Spec review всей фичи. Если review находит finding, reviewer создаёт child follow-up, оставляет review и epic blocked, ждёт исправления и повторяет полный review. Epic запрещено переводить в done, пока review не завершён с нулём unresolved findings.

## Acceptance Criteria

- Все implementation и integration child tasks эпика имеют status done и зелёные declared gates.
- Durable plan v2 сохраняет цель, подход, критерии успеха, ограничения и bounded рабочий контекст без потери после resume/compaction.
- Шаги имеют stable IDs, action, why и done_when; статусы меняются только валидными переходами, а outcome подтверждается evidence.
- Обычный цикл не требует отдельного plan call для старта шага и поддерживает переход к следующему рабочему tool call без промежуточного model round.
- Контрактные изменения требуют reapproval, операционные updates его не сбрасывают; JIT-gated внешние действия требуют отдельного разрешения.
- Prompt projection укладывается в зафиксированный budget, не содержит полных старых логов и не интерпретирует model-authored plan data как system instructions.
- Завершённый plan автоматически архивируется и перестаёт постоянно занимать model context.
- plan-v2-strong-model-review имеет status done после повторного полного review и в эпике нет unresolved review findings.

## Verification Plan

1. Проверить status и acceptance evidence всех child/follow-up tasks через task registry.
2. Запустить focused package tests, integration/e2e plan tests и race tests для session, plan tool, executor/plangate и TUI seams.
3. Измерить happy-path API rounds и размер prompt projection на коротком и максимальном плане.
4. Выполнить финальный Standards+Spec review моделью very_high; при findings повторять fix+review до нулевого результата.

---
id: developer-mode-plan
title: 08 — Показать состояние плана и настройки текущего шага
status: blocked
priority: medium
model_level: medium
task_type: feature
parent_id: developer-mode-readonly
tags:
    - developer-mode
acceptance_criteria:
    - harness plan показывает enabled/disabled, наличие плана, lifecycle/approval, текущий step ID и безопасные telemetry counters.
    - Default policy, loaded policy и применённые ограничения шага различаются с revisions и источниками.
    - Активные skills отражены как имена/состояния применения, не содержимое; тела плана, evidence и working context не дублируются.
    - Read не одобряет, не начинает и не завершает шаг, не меняет plan revision и не возвращает replay/mutation tokens.
    - Отсутствующий/закрытый план и недоступный владелец представлены корректно, без изобретённого default.
verification_plan:
    - 'Fixture disabled/no plan/draft/approved/active/closed: верные безопасные поля и revisions.'
    - Меняются defaults или session step — следующий snapshot показывает актуальное применённое состояние.
    - Collector сам не меняет revision и не утечёт sentinel plan text/evidence/tokens; tool-loop policy работает штатно.
created_at: "2026-09-06T09:07:38.770158Z"
updated_at: "2026-09-06T09:07:38.770158Z"
---

## Body

**Что построить.** Наблюдение за действующим планом и настройками шага через harness без новой возможности управления. Читать epic developer-mode-readonly.

**Blocked by:** developer-mode-headless.

**Фиксированные решения.** Опора на существующий authoritative plan snapshot и telemetry, не парсинг истории tool calls. Разделить default runtime policy и session mutable plan. Без raw планового текста, секретных grants и прав на переходы. Обычная запись evidence/step processing для самого harness вызова остаётся правилом executor; collector не делает дополнительных переходов. В тестах отличать эти два источника изменений.

**Работа.** Task worktree, профильные tests, closeout по epic.

## Acceptance Criteria

- harness plan показывает enabled/disabled, наличие плана, lifecycle/approval, текущий step ID и безопасные telemetry counters.
- Default policy, loaded policy и применённые ограничения шага различаются с revisions и источниками.
- Активные skills отражены как имена/состояния применения, не содержимое; тела плана, evidence и working context не дублируются.
- Read не одобряет, не начинает и не завершает шаг, не меняет plan revision и не возвращает replay/mutation tokens.
- Отсутствующий/закрытый план и недоступный владелец представлены корректно, без изобретённого default.

## Verification Plan

1. Fixture disabled/no plan/draft/approved/active/closed: верные безопасные поля и revisions.
2. Меняются defaults или session step — следующий snapshot показывает актуальное применённое состояние.
3. Collector сам не меняет revision и не утечёт sentinel plan text/evidence/tokens; tool-loop policy работает штатно.

---
id: session-effort-restore
title: 'Сессия: восстановление effort модели при resume'
status: in_progress
priority: high
model_level: medium
task_type: bug
tags:
    - bug
    - effort
    - session
branch: bug/session-effort-restore
worktree_path: .worktrees/session-effort-restore
acceptance_criteria:
    - Возобновление сохранённой сессии восстанавливает сохранённый effort активной модели, а не дефолт
    - Новая сессия по-прежнему стартует с дефолтным/настроенным effort; смена модели внутри сессии не ломается
    - Регрессионный тест пинит восстановление effort из сохранённой сессии
    - CHANGELOG Unreleased; gofmt чист; go test по затронутым пакетам зелёный
verification_plan:
    - 'Красный тест: сессия с не-дефолтным effort → resume → effort восстановлен'
    - 'Проверить смежные пути: новая сессия, смена модели, /effort после resume'
    - go test затронутых пакетов + gofmt
    - CHANGELOG Unreleased
created_at: "2026-09-06T12:28:55.477026Z"
updated_at: "2026-09-06T12:33:44.523711Z"
---

## Body

При запуске сохранённой сессии (resume) не восстанавливается effort
текущей модели — он сбрасывается на default. Найти, где effort хранится
в сессионном состоянии, где сессия сериализуется/восстанавливается и где
дефолт затирает сохранённое значение; починить минимально и застолбить
тестом.

**Started (2026-09-06).** Plan approved; starting trace.

**Note (2026-09-06).** Trace: effort persistится на запись — engine.go:1053 AppendAssistant(msg, model, effort) с обещанием «resumed session can pick up where it left off» (manager.go:207). Теряется на загрузке: у session.Manager есть Model() (manager.go:559, последний assistant entry с моделью), но НЕТ Effort(); NewEngine (engine.go:300-306) резолвит только имя через ResolveModel → свежий каталог-конфиг с базовым ReasoningEffort. Рантайм-раунды читают modelCfg.ReasoningEffort (engine.go:240) → resume работает на default; Controller.Resume (controller.go:2120-2127) честно забирает этот же default в c.modelEffort → и UI показывает default. Фикс: Manager.Effort() (зеркало Model(), тот же anchor-entry, включая пустое значение = default) + в NewEngine применить записанный effort к cfg, если поддержан (паттерн engine_plan_models.go:83-93: ParseReasoningEffort + slices.Contains(ReasoningEfforts) → cfg.ReasoningEffort = level; неподдержанный/пустой — молча базовый). Красный пин: engine-тест resume с записанным «high» против базового «medium».

**Note (2026-09-06).** Red → green. Красный пин TestNewEngineResumeRestoresSessionEffort/recorded_effort_restores падал ровно на баге (expected «high», actual «medium»). Фикс: session.Manager.Effort() + общий helper lastAssistantModelEntry() для Model()/Effort() (manager.go), pass-through agent.Session.Effort() (session.go), применение в NewEngine после резолва модели — stdlib slices.Contains по ReasoningEfforts. ReplaceSession модель не резолвит (только тесты зовут с ResumePath) — вне скопа. TestManagerEffort пинит акцессор (anchor-entry семантика, persist/reload). go test ./... зелёный, gofmt/vet чисты, go.mod не тронут. CHANGELOG Unreleased: Fixed.

## Acceptance Criteria

- Возобновление сохранённой сессии восстанавливает сохранённый effort активной модели, а не дефолт
- Новая сессия по-прежнему стартует с дефолтным/настроенным effort; смена модели внутри сессии не ломается
- Регрессионный тест пинит восстановление effort из сохранённой сессии
- CHANGELOG Unreleased; gofmt чист; go test по затронутым пакетам зелёный

## Verification Plan

1. Красный тест: сессия с не-дефолтным effort → resume → effort восстановлен
2. Проверить смежные пути: новая сессия, смена модели, /effort после resume
3. go test затронутых пакетов + gofmt
4. CHANGELOG Unreleased

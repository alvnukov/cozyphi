---
id: claude-economy-threads-brakes-ledger
title: 'Экономия: треды через --resume, лимит вызовов за ход, дедуп, ledger usage/cost, редакция'
status: todo
priority: medium
model_level: medium
task_type: feature
parent_id: claude-consult-tool
tags:
    - claude
    - economy
    - usage
    - session
acceptance_criteria:
    - thread из результата принимается следующим вызовом и уходит в --resume; реестр тредов виден в op=status и после /resume.
    - 'Четвёртый consult за ход отклоняется с именем лимита; повтор того же брифа отдаёт cached: true; третий одновременный claude-job отклоняется.'
    - Каждый вызов записан в usage с токенами и ценой; usage-pane и футер результата их показывают; секреты в ответе редактируются.
    - make fmt-check lint test rc=0.
verification_plan:
    - go test ./internal/tools/claudetool/... ./internal/usage/... ./internal/session/... -run 'Claude|Thread|Ledger' -race.
    - make fmt-check lint test.
created_at: "2026-09-02T05:13:31.874858Z"
updated_at: "2026-09-02T05:13:31.874858Z"
---

## Body

**Цель.** Каждый вызов Claude дешевле и заметнее: follow-up не переплачивает, цикл не сжигает лимиты, цена видна.

**Треды.** Результат consult возвращает thread = claude session_id; параметр thread → Request.Resume; сессия ведёт реестр тредов (id, mode, label, last_used, calls, tokens) как событие session-лога — переживает /resume; op=status перечисляет треды и итоги за день; неизвестный thread — ошибка со списком известных. Правило в описании тулы: повторная проверка — в тред, без нового брифа.

**Тормоза.** (1) Лимит consult за один ход без ввода пользователя (константа 3, конфиг claude.max_calls_per_turn); превышение — ошибка тулы с именем лимита (не Ask: одинаково работает в headless); сбрасывается любым сообщением пользователя. (2) Дедуп: hash(mode, отрендеренный бриф, thread) → в пределах сессии повторный вызов отдаёт ответ из result_path с пометкой cached: true. (3) Отдельный кап одновременных claude-job (claude.max_concurrent, дефолт 2) — проверка в тule через Manager.List перед Spawn.

**Ledger.** internal/usage получает запись на вызов: mode, model, tokens in/out/cache read/cache create, cost_usd, duration, turns; usage-pane показывает строку claude (сегодня/сессия); футер результата печатает те же цифры. Текст ответа проходит redact перед тем, как стать Content тулы и строкой транскрипта.

**Тесты.** argv содержит --resume; реестр тредов переживает resume; лимит за ход и сброс; дедуп; кап параллельности; запись ledger; редакция секрета в ответе fake-бинаря.

**Зависит от:** claude-tool-and-gate.

## Acceptance Criteria

- thread из результата принимается следующим вызовом и уходит в --resume; реестр тредов виден в op=status и после /resume.
- Четвёртый consult за ход отклоняется с именем лимита; повтор того же брифа отдаёт cached: true; третий одновременный claude-job отклоняется.
- Каждый вызов записан в usage с токенами и ценой; usage-pane и футер результата их показывают; секреты в ответе редактируются.
- make fmt-check lint test rc=0.

## Verification Plan

1. go test ./internal/tools/claudetool/... ./internal/usage/... ./internal/session/... -run 'Claude|Thread|Ledger' -race.
2. make fmt-check lint test.

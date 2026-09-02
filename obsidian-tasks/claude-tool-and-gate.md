---
id: claude-tool-and-gate
title: 'Тула claude: схема, описание-экономия, ActionClaude в gate, регистрация в engine, строка в system prompt'
status: todo
priority: high
model_level: high
task_type: feature
parent_id: claude-consult-tool
tags:
    - claude
    - tool
    - permission
    - prompt
acceptance_criteria:
    - Тула claude зарегистрирована только в сессиях с Jobs и включённым, найденным claude; sub-agents её не имеют; system prompt содержит строку маршрутизации только при регистрации.
    - 'Gate: read-only режимы и status — Allow; implement — Ask с именем каталога; в headless-strict implement отклоняется.'
    - consult возвращает структурированный ответ с thread/usage/cost или {job_id, hint} при истечении ожидания; результат ограничен 12 KB с result_path.
    - Описание тулы содержит таблицу режимов, правила брифа, список «когда не звать» и правило тредов; цифры лимитов берутся из констант.
    - make fmt-check lint test rc=0.
verification_plan:
    - go test ./internal/tools/claudetool/... ./internal/permission/... ./internal/agent/... -run 'Claude' -race.
    - 'Ручной прогон в TUI: consult mode=architect с двумя files → ответ; mode=implement → появляется Ask.'
    - make fmt-check lint test.
created_at: "2026-09-02T05:13:31.874227Z"
updated_at: "2026-09-02T05:13:31.874227Z"
---

## Body

**Цель.** Шов, на котором держится эпик: что модель видит, когда зовёт Claude, и что ей запрещено.

**Тула.** internal/tools/claudetool, имя claude. Параметры: op (consult по умолчанию | status), mode (enum шести режимов), task, deliverable (оба обязательны для consult — структура брифа заставляет дешёвую модель сформулировать вопрос до вызова), context, files[{path, lines?}], attach_plan, attach_diff, thread, workdir, worktree, timeout_sec, label. consult: собирает Brief (вложения — через Deps.Attach, реализуются в claude-attachments-plan-and-diff; здесь — files и текст), Spawn(Backend=claude), блокирует до timeout_sec (дефолт из пресета), при истечении возвращает {job_id, status: running, hint: agent_wait}; результат ограничен 12 KB (как agentSummaryLimit) с result_path; в ответе всегда thread, usage, cost, turns, duration. status: Client.Status + список тредов (реестр появится в economy — здесь пусто). DetailFromArgs: «claude <mode>: <label|task…>». Deps{Manager, ParentID, WorkDir, Client, Presets, Attach}.

**Описание тулы — главный рычаг экономии.** Таблица режимов с ценой и когда какой; правила брифа (что известно — с путями; выдержки, а не файлы; deliverable явно); список «когда НЕ звать» (grep/lsp, рутинные правки, компакция, повторный бриф вместо thread, exploration без вопроса); правило тредов; лимиты рендерятся из констант.

**Permission.** ActionClaude в policy.go/extract.go/gate.go: extract читает mode и workdir; gate: op=status и режимы с AllowsFS=false → Allow; implement → Ask с текстом «claude implement will edit files under <dir> (worktree: yes/no): <label>»; foldMode без изменений — в headless-strict implement невозможен без явного allow-all, как у sub-agents. Хуки видят тулу как любую другую.

**Регистрация.** EngineOpts.Claude (клиент + пресеты) → engine регистрирует тулу только при Jobs != nil и Claude != nil; ChildSpec тулу не содержит (дети без claude, как без agent_* и watch); cmd/run.go и internal/tui/controller подключают её, когда конфиг enabled и бинарь резолвится; иначе одна строка warning и тула не регистрируется (в отличие от lsp: нерабочая тула — лишние токены схемы). system-prompt.tmpl: одна условная строка маршрутизации (архитектура, план до approve, diff критичного кода, диагноз после повторов, исследование после провала — → claude; бриф, а не пересказ) — по тому же механизму, что строка watch.

**Тесты.** матрица extract/gate по режимам и op; строгий декод входа; engine регистрирует условно (образец engine_lsp_test.go); sub-agent не видит тулу; consult с истёкшим timeout возвращает job_id; результат обрезается с result_path.

**Зависит от:** claude-modes-and-brief, claude-job-backend.

## Acceptance Criteria

- Тула claude зарегистрирована только в сессиях с Jobs и включённым, найденным claude; sub-agents её не имеют; system prompt содержит строку маршрутизации только при регистрации.
- Gate: read-only режимы и status — Allow; implement — Ask с именем каталога; в headless-strict implement отклоняется.
- consult возвращает структурированный ответ с thread/usage/cost или {job_id, hint} при истечении ожидания; результат ограничен 12 KB с result_path.
- Описание тулы содержит таблицу режимов, правила брифа, список «когда не звать» и правило тредов; цифры лимитов берутся из констант.
- make fmt-check lint test rc=0.

## Verification Plan

1. go test ./internal/tools/claudetool/... ./internal/permission/... ./internal/agent/... -run 'Claude' -race.
2. Ручной прогон в TUI: consult mode=architect с двумя files → ответ; mode=implement → появляется Ask.
3. make fmt-check lint test.

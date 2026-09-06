---
id: developer-mode-agents
title: 12 — Показать метаданные агентов, заданий и watches
status: done
priority: medium
model_level: medium
task_type: feature
parent_id: developer-mode-readonly
tags:
    - developer-mode
acceptance_criteria:
    - agents показывает включённость, role pins/inheritance, доступные ограничения и безопасные состояния заданий текущей сессии.
    - diagnostics.watches показывает поддержку, лимиты, число/состояния наблюдений своей сессии; headless явно unavailable/not_applicable.
    - Developer-доступ детей не наследуется; snapshot не расширяет доступ к заданиям других сессий и не раскрывает prompt/result/transcript.
    - Не выводятся shell commands, labels с произвольным чувствительным текстом, raw output и errors; только allowlisted metadata.
    - Чтение не запускает/останавливает задачи или watches, не делает recovery/повторный spawn и не ждёт завершения job.
verification_plan:
    - Fixtures role pin/inherit/stale pin, job states и watch limits; headless без watch manager.
    - 'Две сессии и child: snapshot показывает только разрешённый scope; нет чужих prompts/results.'
    - Spies spawn/cancel/recover/watch-start/stop/wait не вызываются; sentinel commands/labels/errors redacted.
created_at: "2026-09-06T09:08:35.746978Z"
updated_at: "2026-09-06T20:30:30.734898Z"
---

## Body

**Что построить.** Сквозное наблюдение execution metadata агентов/заданий и watches. Читать epic developer-mode-readonly.

**Blocked by:** developer-mode-headless.

**Scope.** Agents-категория для roles/jobs, diagnostics.watches для watches; не дублировать одно состояние в двух категориях. Использовать bound owner/session identity, а не обход всего process manager. Process-wide limits можно показывать агрегатами без чужого содержимого. Role model source разрешается существующим resolver, а не новым выбором модели. Parent developer flag не является child capability.

**Работа.** Task worktree; профильные fixtures без процессов/моделей; closeout по epic.

**Сделано.** Две новые категории каталога. `agents` (internal/diag/agents.go) отвечает из job-менеджера, к которому привязана сессия: включённость и наличие менеджера сведены в один словарь состояний — disabled, no_manager, depth_reached, saturated, running, idle, — чтобы «выключено», «некому запускать» и «всё занято» не читались одинаково. Роли разделены на три ответа: словарь ролей, роли, которые эта сессия ещё может запустить, и роль, которой она сама является (у sub-agent'а нет spawn-инструмента вовсе). Пин модели по роли показан рядом с тем, во что он разрешился, — устаревший пин видно деградирующим в наследование, а не выглядящим как отработавшая модель. Потолок вложенности показан против собственной глубины, потолок параллелизма — против process-wide загрузки и доли этой сессии (spawn отказывают из-за процесса, а сессия, видящая только себя, не отличила бы это от бага). Назначения считаются по роли и по статусу и больше ни по чему: ни prompt, ни description, ни summary, ни result, ни workdir, ни job id. Developer-доступ заявлен как то, что spawn не передаёт.

`diagnostics` (internal/diag/diagnostics.go) открыт на watches: наличие менеджера (у headless его нет — это не то же самое, что сессия без единого watch), бюджеты и остаток живых слотов, счётчики запущенных и ещё живых, форма каждого watch со streaming-командой, разделённой по триггеру (stream:line / stream:exit / poll / timer), самый частый оставшийся интервал (watches.cadence), число доставленных событий и исходы с отделением failed и flooded от просто закончившихся. Только формы, счётчики и словари — ни label, ни command, ни match, ни workdir, ни текста события или ошибки.

Наблюдатели: internal/job/observe.go (Observe(m, owner) под одним локом, только память — finished job живёт на диске и в ответ не попадает), internal/watch/observe.go, project.AgentModels.Observe(). Проводка: controller (agentsState/agentDepth/watchState) и cmd/run.go. Чтение не запускает и не отменяет назначения, не стартует и не останавливает watches, не читает store и не сканирует файловую систему.

**Тесты.** internal/diag/agents_test.go (10), internal/diag/diagnostics_test.go (11), internal/job/observe_test.go (7, под -race), internal/watch/observe_test.go (9, под -race), internal/tui/controller/developer_mode_agents_test.go (6), internal/project/agent_models_observe_test.go (3), cmd/run_developer_agents_test.go (4). Sentinel-проверки двусторонние: сперва доказано, что владелец (mgr.List()[0].Err, c.watches.List()[0].Label) sentinel всё ещё держит, и только потом — что в наблюдение он не попал. В TUI-тестах watches только timer-формы, чтобы не порождать процессов.

**Попутно.** cmd/run_developer_hooks_test.go не компилировался с fd6dec0 (тикет 11): вызывался text("done") при хелпере doneText(). Пакет cmd из-за этого не собирался на main. Исправлено и здесь, и параллельно на main коммитом 1785904.

**Gates.** Только изменённые пакеты: go test ./internal/diag ./internal/job ./internal/watch ./internal/project ./internal/tui/controller ./cmd — ok; повторно после merge на main — ok. golangci-lint по тем же путям — чисто.

**Closeout.** bac104d на feature/developer-mode-agents, merge --no-ff 36cf2b3 в main (конфликт CHANGELOG разрешён сохранением обеих сторон). Не пушилось.

## Acceptance Criteria

- agents показывает включённость, role pins/inheritance, доступные ограничения и безопасные состояния заданий текущей сессии.
- diagnostics.watches показывает поддержку, лимиты, число/состояния наблюдений своей сессии; headless явно unavailable/not_applicable.
- Developer-доступ детей не наследуется; snapshot не расширяет доступ к заданиям других сессий и не раскрывает prompt/result/transcript.
- Не выводятся shell commands, labels с произвольным чувствительным текстом, raw output и errors; только allowlisted metadata.
- Чтение не запускает/останавливает задачи или watches, не делает recovery/повторный spawn и не ждёт завершения job.

## Verification Plan

1. Fixtures role pin/inherit/stale pin, job states и watch limits; headless без watch manager.
2. Две сессии и child: snapshot показывает только разрешённый scope; нет чужих prompts/results.
3. Spies spawn/cancel/recover/watch-start/stop/wait не вызываются; sentinel commands/labels/errors redacted.

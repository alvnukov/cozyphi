---
id: subagent-row-progress
title: A — Живая строка сабагента в родителе и итог в том же блоке
status: done
priority: high
model_level: high
task_type: feature
parent_id: subagent-panel-ux
tags:
    - agents
    - tui
    - transcript
branch: feature/subagent-row-progress
worktree_path: .worktrees/subagent-row-progress
acceptance_criteria:
    - Интерактивный раннер эмитит job.Progress на каждое ToolData ребёнка так же, как EngineRunner; SubagentStore родителя заполняется при интерактивных детях.
    - Строка spawn называется role(описание) с прежними суффиксами skills/model, при работе показывает · N tools · elapsed (тик через WakeIn), после завершения — глиф ✓/✗/■, замороженные счётчики и summary итога в том же блоке.
    - Доставленный outcome обновляет блок живьём (bus-сообщение) и после resume (replay распознаёт delivery по DeliveryID вместо StripReminders); при отсутствии строки spawn — локальная строка с итогом.
    - Модель получает тот же <system-reminder>; agent_wait и headless не меняются; никаких токенов в строке.
verification_plan:
    - Тест раннера/контроллера: интерактивный ребёнок публикует JobProgressMsg с ParentToolUseID; дедуп прежний.
    - Тесты mapper: заголовок role(описание), счётчик и elapsed при running, глиф и summary при done/error/stopped.
    - Тест replay: история с outcome delivery → блок с summary, reminder не в тексте пользователя.
    - Гейты по изменённым пакетам: gofumpt/golines, go build ./cmd, go test -race по agent/job/controller/transcript/session, один golangci-lint run по изменённым пакетам.
created_at: "2026-09-06T18:40:00Z"
updated_at: "2026-09-06T20:00:00Z"
---

## Body

Фаза A эпика subagent-panel-ux; контракт — пункты 1 и 2 «Target contract» в `AGENTS_DESIGN.md`. Код только в worktree на своей ветке; ledger на main.

**Стартовые точки.** `internal/tui/controller/children.go` (`interactiveRunner.Run` не вызывает `env.OnProgress`); `internal/agent/engine_runner.go` (единственный вызов `OnProgress` на `session.ToolData`); `internal/job/manager.go` (заполняет JobID/ParentID/OwnerID/ParentToolUseID); `internal/tui/controller/controller.go` `startJobProgress`; `internal/tui/transcript/{mapper.go,subagent_store.go,pane.go,replay.go}`; `internal/components/block/agent_block.go`; `internal/agent/outcomes.go` `AcceptOutcome`; `internal/tui/controller/outcomes.go` `deliverOutcomes`; `internal/tools/agenttool/agent.go` `spawnDetail`.

**Done (2026-09-06).** Слито в main (325c6a4): интерактивный раннер эмитит job.Progress через assignmentHooks; строка spawn — role(описание) с `· N tools · elapsed`, глифы ✓/✗/■; ChildOutcomeMsg обновляет блок живьём; replay распознаёт DeliveryID `:terminal` и рисует локальную строку agent_spawn (title из аргументов spawn в истории, иначе job id). Отклонения: replay не кормит SubagentStore (строка уже несёт summary); после холодного resume elapsed не показывается (Outcome без таймингов — не выдумываем). Гейты по изменённым пакетам зелёные.

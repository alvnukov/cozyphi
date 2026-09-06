---
id: subagent-agents-pane
title: C — /agents: список детей сессии на шаблоне watchpane
status: done
priority: medium
model_level: high
task_type: feature
parent_id: subagent-panel-ux
tags:
    - agents
    - tui
branch: feature/subagent-agents-pane
worktree_path: .worktrees/subagent-agents-pane
acceptance_criteria:
    - Команда /agents (и палитра) открывает полноэкранную панель на шаблоне internal/tui/watchpane; строки — все дети текущей сессии из job.Manager (фильтр по ParentID сессии), работающие сверху, завершённые ниже, каждая со статусом, role(описание), tools, elapsed и однострочным summary/ошибкой.
    - Enter на работающем или retained ребёнке открывает его View текущим экраном (тот же путь, что панель под композером); на ребёнке без retained View — уведомление, откуда читать результат (result.md); x останавливает работающего после y/n; Esc/q закрывает.
    - Футер вместо «N jobs» показывает «N agents»; подсказки только из каталога keys (новый scope), строка закреплена в keys_test; DESIGN.md — adoption.
    - Никаких токенов; поля, которых нет в Meta, не выдумываются.
verification_plan:
    - Тесты пакета: порядок running→done, формат строк, Enter/x/Esc, фильтр по сессии.
    - Тесты keys и footer на новую подпись.
    - Гейты по изменённым пакетам: gofumpt/golines, go build ./cmd, go test -race, один golangci-lint run.
created_at: "2026-09-06T19:35:00Z"
updated_at: "2026-09-06T21:45:00Z"
---

## Body

Фаза C эпика subagent-panel-ux — пункт 6 «Target contract» в `AGENTS_DESIGN.md`. Стартует после слияния B2 (нужен путь «открыть ребёнка экраном»).

**Стартовые точки.** `internal/tui/watchpane/pane.go` (шаблон); `internal/tui/commands/builtins.go` (регистрация `/watches` → по образцу `/agents`); `internal/tui/sessions/view.go` (`ShowWatches`, лестница `Handle`, `Draw` overlay-слой, `Focus`); `internal/tui/footer/footer.go` `jobLabel`; `internal/job/manager.go` `List`/`Get` (глобальный список — фильтровать по `Meta.ParentID == SessionID()` родителя), `Meta` (Role, Description, Status, StartedAt, FinishedAt, OutcomeSummary, Error, ResultPath); `internal/tools/agenttool/agent.go` (`agent_cancel` → `CancelForOwner`).

**Started (2026-09-06).** B2 слита; работа в `.worktrees/subagent-agents-pane` на `feature/subagent-agents-pane`, параллельно с соседней фазой.

**Done (2026-09-06).** Слито в main (a6cd45e): пакет `internal/tui/agentlist` (имя вместо agentpane — слишком похоже на agentpanel), `/agents` в командах и палитре, `Controller.ChildJobs` (фильтр Manager.List по ParentID), `Family.Agents/OpenAgent/StopAgent`, футер `N agents`, scope `agent-list` в keys, doc/tui.md, DESIGN.md, CHANGELOG. Enter открывает только retained-ребёнка; для отпущенного — notice с путём результата. Гейты по изменённым пакетам зелёные.

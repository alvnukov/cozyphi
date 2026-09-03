---
id: claude-job-backend
title: ClaudeRunner как второй адаптер job.Runner; Meta.Backend; прогресс и result.md
status: todo
priority: high
model_level: medium
task_type: feature
parent_id: claude-consult-tool
tags:
    - claude
    - job
    - runner
acceptance_criteria:
    - Job с Backend=claude проходит starting→running→completed на fake-бинаре; result.md содержит ответ и футер usage; events.jsonl содержит поток.
    - agent_wait/agent_list/agent_cancel работают на claude-job без изменений своего кода; cancel завершает процесс.
    - Meta.Backend/Options персистятся и переживают recovery; неизвестный бэкенд отвергается с понятной ошибкой.
    - make fmt-check lint test rc=0.
verification_plan:
    - go test ./internal/job/... ./internal/agent/... -run 'Claude|Backend' -race.
    - make fmt-check lint test.
created_at: "2026-09-02T05:13:31.87395Z"
updated_at: "2026-09-02T05:13:31.87395Z"
---

## Body

**Цель.** Консультация — это job: тот же менеджер, каталог ~/.cozyphi/jobs/<id>/, agent_wait/agent_list/agent_cancel, таймаут, recovery, прогресс в TUI.

**Что сделать.** (1) job.Meta и SpawnRequest получают Backend (""|engine → engine, claude) и Options json.RawMessage — непрозрачные для job опции бэкенда (для claude: mode, thread, workdir/worktree флаги, label); validate принимает только известные бэкенды; Info/agent_list показывают backend, когда он не engine. Prompt в Meta — уже отрендеренный бриф (аудит в meta.json). (2) internal/agent/claude_runner.go: ClaudeRunner{Client, Presets, Ledger-хук позже} реализует job.Runner: читает Options, строит Request из пресета, запускает Client.Run с ctx job'а; tool_use события → env.OnProgress(job.Progress{Name: "claude:"+tool, Status, Detail}) и env.Log; result.md = Answer.Render() + футер (session_id, turns, duration, tokens in/out/cache, cost); ошибка/IsError → job failed с текстом причины; отмена → cancelled, таймаут → timed_out — семантика менеджера не меняется. (3) agent.NewJobManager собирает backendRunner{engine: EngineRunner, claude: ClaudeRunner}, диспетчер по meta.Backend; неизвестный бэкенд → failed с actionable-ошибкой; nil ClaudeRunner → бэкенд claude отвергается на Spawn. (4) Слоты MaxConcurrent общие; отдельный кап для claude — в задаче economy.

**Тесты.** Manager.Spawn с Backend=claude на fake-бинаре: статусы, прогресс, result.md с футером, cancel убивает процесс, recovery после «рестарта» помечает failed; agent_* тулы без изменений; validate отвергает неизвестный бэкенд.

**Зависит от:** claude-runner-proc-adapter, claude-modes-and-brief.

## Acceptance Criteria

- Job с Backend=claude проходит starting→running→completed на fake-бинаре; result.md содержит ответ и футер usage; events.jsonl содержит поток.
- agent_wait/agent_list/agent_cancel работают на claude-job без изменений своего кода; cancel завершает процесс.
- Meta.Backend/Options персистятся и переживают recovery; неизвестный бэкенд отвергается с понятной ошибкой.
- make fmt-check lint test rc=0.

## Verification Plan

1. go test ./internal/job/... ./internal/agent/... -run 'Claude|Backend' -race.
2. make fmt-check lint test.

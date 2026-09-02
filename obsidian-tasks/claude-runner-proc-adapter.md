---
id: claude-runner-proc-adapter
title: 'internal/claude: конфиг, резолв бинаря, запуск claude -p и разбор stream-json'
status: todo
priority: high
model_level: medium
task_type: feature
parent_id: claude-consult-tool
tags:
    - claude
    - runner
    - proc
acceptance_criteria:
    - 'LoadConfig ведёт себя как lsp: дефолты при отсутствии файла, fail closed на небезопасном файле и неизвестных ключах, ошибки без содержимого файла и argv.'
    - Client.Run на fake-бинаре возвращает Result с SessionID, Usage, CostUSD, NumTurns и StructuredOutput из записанной фикстуры; tool_use события приходят в OnEvent по мере потока.
    - Отмена ctx завершает процесс и его детей; таймаут даёт ошибку с хвостом stderr; prompt уходит в stdin, argv не содержит prompt.
    - 'Отсутствие бинаря не ошибка на старте: Status сообщает InstallHint, Run — понятную ошибку.'
    - make fmt-check lint test rc=0.
verification_plan:
    - go test ./internal/claude/... -race.
    - Записать фикстуру реальным claude и один раз прогнать COZYPHI_CLAUDE_SMOKE=1 go test ./internal/claude/ -run Smoke.
    - make fmt-check lint test.
created_at: "2026-09-02T05:13:31.873191Z"
updated_at: "2026-09-02T05:13:31.873191Z"
---

## Body

**Цель.** Глубокий модуль internal/claude без модель-facing тулы: харнесс владеет процессом claude так же, как gopls в internal/lsp.

**Что сделать.** (1) Config{Enabled, Command []string, Env []string} из ~/.cozyphi/claude.json — загрузчик по образцу internal/lsp/config.go: отсутствующий файл = дефолты (Command: ["claude"]), symlink/чужой владелец/group- или world-writable/неизвестные ключи — fail closed с sanitized-ошибкой; если internal/configfile уже даёт общий безопасный загрузчик — использовать его, а не копировать. Project-local конфиг не читается. (2) Resolve: первый элемент Command — абсолютный путь или basename через ~/.cozyphi/bin, затем PATH, никогда cwd; Version(ctx) = claude --version; Status{Configured, Installed, Path, Version, InstallHint}. (3) Client.Run(ctx, Request) (Result, error): Request{Prompt, Model, Effort, Tools, AllowedTools, DisallowedTools, PermissionMode, AppendSystemPrompt, JSONSchema, Resume, Dir, AddDirs, SettingSources, StrictMCP, DisableSlashCommands, NoSessionPersistence, Timeout, OnEvent}; argv строится без шелла: claude -p --output-format stream-json (+ --verbose, если версия его требует для stream-json в -p) и только те флаги, что заданы; prompt подаётся через stdin, не через argv (ps и лимит длины). (4) Разбор потока построчно через proc.Spec.Stream: события assistant (tool_use → OnEvent{Kind: tool_use, Name, Detail}; text → Kind: text), финальный result → Result{Text, StructuredOutput json.RawMessage, SessionID, Subtype, IsError, NumTurns, DurationMS, CostUSD, Usage{Input, Output, CacheRead, CacheCreate}}; неизвестные типы событий игнорируются, кривая строка — предупреждение в Result.Warnings, не ошибка; отсутствие result при exit 0 — ошибка с хвостом stderr. (5) Env: sanitized наследование как у lsp (переиспользовать его санитайзер, при необходимости подняв в proc), минус COZYPHI_* ключи провайдеров, плюс Config.Env. (6) Отмена ctx = kill process tree (proc это умеет); retention вывода в памяти ограничен (хвост), полный поток отдаётся только через OnEvent.

**Фикстура.** stream-json меняется между версиями: записать один реальный прогон установленной claude (2.1.258) в testdata/stream-json/*.jsonl (короткий prompt, без инструментов) и строить парсер по нему, а не по памяти. Smoke-тест с реальным бинарём — только под env COZYPHI_CLAUDE_SMOKE=1 (образец internal/lsp/smoke_real_test.go).

**Тесты.** Fake-бинарь (скрипт в testdata или re-exec паттерн, как в lsp fake_test.go): happy path, tool_use события, is_error, отмена посреди потока, таймаут, невалидные строки, огромный вывод; конфиг: матрица fail-closed; argv: golden на полный Request и на пустой.

**Границы.** Никаких режимов, брифов и тул — это следующие задачи. Зависимостей нет.

## Acceptance Criteria

- LoadConfig ведёт себя как lsp: дефолты при отсутствии файла, fail closed на небезопасном файле и неизвестных ключах, ошибки без содержимого файла и argv.
- Client.Run на fake-бинаре возвращает Result с SessionID, Usage, CostUSD, NumTurns и StructuredOutput из записанной фикстуры; tool_use события приходят в OnEvent по мере потока.
- Отмена ctx завершает процесс и его детей; таймаут даёт ошибку с хвостом stderr; prompt уходит в stdin, argv не содержит prompt.
- Отсутствие бинаря не ошибка на старте: Status сообщает InstallHint, Run — понятную ошибку.
- make fmt-check lint test rc=0.

## Verification Plan

1. go test ./internal/claude/... -race.
2. Записать фикстуру реальным claude и один раз прогнать COZYPHI_CLAUDE_SMOKE=1 go test ./internal/claude/ -run Smoke.
3. make fmt-check lint test.

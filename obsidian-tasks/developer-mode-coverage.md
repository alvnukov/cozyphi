---
id: developer-mode-coverage
title: 16 — Проверить полноту, секреты и read-only поведение developer mode
status: done
priority: medium
model_level: medium
task_type: test
parent_id: developer-mode-readonly
tags:
    - developer-mode
acceptance_criteria:
    - 'Матрица учитывает все supported config fields, CLI overrides, известные ENV overrides, imports/session overrides и существенные встроенные лимиты: каждое сопоставлено с catalog либо явно обоснованным исключением.'
    - В финальном catalog нет временного not_implemented из 01; unavailable допустим только как реальное состояние/обоснованное ограничение, не замена неподключённого collector.
    - Сквозная TUI/headless матрица подтверждает флаг, отсутствие включения через config/ENV/resume/UI, отсутствие наследования children и прямой отказ без capability.
    - Sentinel secrets в keys/URL/args/env/paths/errors/contents не появляются в tool results, error paths, transcript и audit.
    - Spy adapters подтверждают отсутствие дополнительных reload/probe/start/scan/write; штатные pre/post hooks и evidence самого tool call не ошибочно считаются collector side effects.
    - Concurrency/cancellation/truncation/scope tests проверяют общий interface с несколькими owners; выполняются только scoped проверки.
    - Новые крупные недоделки/баги оформлены отдельными medium/low задачами и блокируют приёмку, а не незаметно реализуются в этом тестовом билете.
verification_plan:
    - 'Составить и автоматически проверить coverage inventory: категории, поля, sources, exceptions; сопоставить с collectors.'
    - Запустить scoped end-to-end matrix через настоящий harness interface на fixture TUI/headless engines.
    - Проверить security sentinel output channels, side-effect spies, scope/cancellation/races; приложить точные команды и результаты к задаче.
created_at: "2026-09-06T09:09:49.306932Z"
updated_at: "2026-09-07T02:07:31.227064Z"
---

## Body

**Что построить.** Приёмочная доказательная матрица полного read-only developer mode, а не новая реализация всех collectors. Читать epic developer-mode-readonly и результаты задач 02–15.

**Blocked by:** developer-mode-tui, developer-mode-model, developer-mode-providers, developer-mode-permissions, developer-mode-tools, developer-mode-context, developer-mode-plan, developer-mode-mcp, developer-mode-lsp, developer-mode-hooks, developer-mode-agents, developer-mode-storage, developer-mode-ui, developer-mode-diagnostics.

**Фиксированные решения.** Каждая collector-задача уже поставляет собственные тесты; здесь проверяется совместный контракт и полнота инвентаризации, не пишется второй набор внутренних unit tests. Проверяемая coverage table должна ловить незарегистрированные новые настройки через сопоставление с известными loaders/schema/flags, а runtime-only limits иметь явный перечень. Не требовать reflection dump конфигов. На исходниках архитектуру массово не менять. Допустимые исключения объясняют отсутствие информации; нельзя закрыть обещание полноты, назвав все ещё не реализованные поля unavailable.

**Работа.** Task worktree. Запускать только пакеты/сценарии developer mode и затронутых owners, без make test/lint всего repo. Fixture runtime без реальных credentials, провайдеров, servers и OS notifications. Closeout по epic.

**Сделано.** Коммит f4a9efc, слит в main. Три файла в `internal/tui/controller`.

`developer_mode_surface_test.go` — сканер настраиваемой поверхности по AST исходников, без reflection и без сборки. Три источника ровно так, как их находят сами loaders: yaml-теги схем (плюс json внутри `internal/project` — это persisted UIState), литералы в `os.Getenv`/`os.LookupEnv`/`firstEnv`, литеральные пути `configfile.Set/Lookup`, и длинные флаги в пакете `cmd`, который разбирает argv вручную. Найдено 150 config, 19 ENV, 13 CLI.

`developer_mode_coverage_test.go` — инвентаризация. На каждую настройку один из трёх ответов: поле каталога, обоснование «никогда» или заведённый тикет. Шесть schema-подобных структур в `cmd` и `internal/tasks` пропущены явно, с причиной; сканер проверяет, что пропуск ещё что-то ловит. Runtime-поверхность — явный перечень из 19 встроенных лимитов и session-overrides, как требуют фиксированные решения. Четыре проверки: настройка↔инвентаризация в обе стороны, каждое reported-поле есть в живом каталоге, ни одна категория не not_implemented и ни одна не unavailable при полной обвязке, каждый gap называет тикет.

`developer_mode_acceptance_test.go` — сквозная приёмка. Сессия, у которой секрет посажен в ключ и адрес провайдера, ключ, адрес и командную строку голосового бэкенда, команду и устройство захвата, bash- и mcp-правила пользователя, аргументы и окружение mcp-сервера, командную строку хука, содержимое памяти и COZYPHI_API_KEY. Через настоящий `harnesstool` задаются все вопросы, которые он принимает, и пять, которые он обязан отвергнуть; ответы, отказы, записанный на диск transcript и audit-запись ищутся вместе. Контроль на невырожденность: alpha, autopilot, vault, guard-bash, whisper-1 в ответах есть. Отдельно — отпечаток деревьев HOME/cwd/session dir до и после двух полных обходов (хук не выполнялся, сервер не стартовал, журнал не открывался, хранилища не переписаны), неизменность набора инструментов, плана и каталога; гонка трёх сессий по двенадцати горутинам с проверкой, что каждая отвечает своей session.id; отменённый контекст на обеих формах snapshot и на explain.

**Новые тикеты.** developer-mode-web-policy (medium) — вся секция web, двадцать полей, включая ключ Google CSE, без единого представления в каталоге. developer-mode-settings-tail (low) — пятнадцать настроек голоса, два журнальных ENV и три флага headless-запуска. Оба дети эпика и блокируют его приёмку.

**Найдено попутно.** `voice.auto_send` снят: загрузка отвергает ключ отдельной ошибкой, поэтому он не gap, а withheld.

**Команды и результаты.** `go test ./internal/tui/controller/ -race -count=1` → ok 87.582s. `go test ./cmd/ -run 'Developer|Headless' -count=1` → ok 6.263s. `go test ./internal/diag/ ./internal/tools/harnesstool/ -count=1` → ok 1.416s / ok 0.897s. `golangci-lint run ./internal/tui/controller/` → 0 issues. Проверка на невырожденность: подмена secretMark на «alpha» даёт 5 срабатываний по трём каналам; удаление трёх записей из инвентаризации ловится в config, env и flag по отдельности; подмена ключа на несуществующий ловится сверкой с живым каталогом.

## Acceptance Criteria

- Матрица учитывает все supported config fields, CLI overrides, известные ENV overrides, imports/session overrides и существенные встроенные лимиты: каждое сопоставлено с catalog либо явно обоснованным исключением.
- В финальном catalog нет временного not_implemented из 01; unavailable допустим только как реальное состояние/обоснованное ограничение, не замена неподключённого collector.
- Сквозная TUI/headless матрица подтверждает флаг, отсутствие включения через config/ENV/resume/UI, отсутствие наследования children и прямой отказ без capability.
- Sentinel secrets в keys/URL/args/env/paths/errors/contents не появляются в tool results, error paths, transcript и audit.
- Spy adapters подтверждают отсутствие дополнительных reload/probe/start/scan/write; штатные pre/post hooks и evidence самого tool call не ошибочно считаются collector side effects.
- Concurrency/cancellation/truncation/scope tests проверяют общий interface с несколькими owners; выполняются только scoped проверки.
- Новые крупные недоделки/баги оформлены отдельными medium/low задачами и блокируют приёмку, а не незаметно реализуются в этом тестовом билете.

## Verification Plan

1. Составить и автоматически проверить coverage inventory: категории, поля, sources, exceptions; сопоставить с collectors.
2. Запустить scoped end-to-end matrix через настоящий harness interface на fixture TUI/headless engines.
3. Проверить security sentinel output channels, side-effect spies, scope/cancellation/races; приложить точные команды и результаты к задаче.

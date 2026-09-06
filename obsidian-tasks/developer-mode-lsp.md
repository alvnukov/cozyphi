---
id: developer-mode-lsp
title: 10 — Наблюдать LSP без запуска, синхронизации и diagnostics
status: done
priority: medium
model_level: medium
task_type: feature
parent_id: developer-mode-readonly
tags:
    - developer-mode
acceptance_criteria:
    - integrations.lsp показывает configured/installed/running, workspace, поддерживаемые операции и безопасный source/state.
    - Не вызываются server start, download, workspace sync, Query или diagnostics ради самого снимка.
    - Сохраняются trust/validation rules LSP config; developer mode не загружает файл обходным способом.
    - Неподдержанный язык, выключенный manager, закрытие и неизвестная freshness представлены явно.
    - DTO не раскрывает secret-bearing args/env/raw process errors; snapshots detached и безопасны при смене состояния.
verification_plan:
    - 'Fixture manager: absent/configured/installed/running/closed/unsupported, корректный workspace.'
    - Spies start/download/query/sync не вызываются при harness чтении.
    - Race-проверка затронутого status path при shutdown и tests secret redaction.
created_at: "2026-09-06T09:08:35.744764Z"
updated_at: "2026-09-06T15:01:07.503684Z"
---

## Body

**Что построить.** Метаданные LSP runtime доступны через harness integrations без воздействия на language server. Читать epic developer-mode-readonly.

**Blocked by:** developer-mode-headless.

**Фиксированные решения.** Наблюдать manager и его known status; не пользоваться Query как shortcut к status, если это может запускать server или синхронизацию. Installed можно сообщать по уже известному состоянию владельца; иначе unavailable, а не probe с side effects. Diagnostics freshness не обещается без наблюдения владельца. Не менять LSP ownership/lifetime.

**Работа.** Task worktree; профильные unit/integration tests без настоящего language server; closeout по epic.

**Сделано.** Мердж da6ce49 в main.

`internal/lsp/observe.go` — шов на стороне владельца, зеркало `mcp/observe.go`. `Observe` читает поля manager под его же mutex: `closed`, живые клиенты, `workspace`, `config.Gopls.Command`. Query не вызывается вообще — `languagesStatus()` не переиспользован намеренно. `ObserveOpen(config, err)` снимает Disabled/Failed на месте вызова: `Open` отдаёт `(nil, nil)` и для выключенной подсистемы, и nil для отказа — по одному manager эти два случая не различить.

`internal/diag/integrations_lsp.go` — 8 ключей `lsp.*`, три закрытых словаря (`LSPLifecycle` 9 значений, `LSPInstall` 3, `LSPStart` 6) и `LSPServerFacts`/`LSPState`. `lifecycle` разводит шесть разных ситуаций с одним симптомом; `languages`/`servers` — три сжимающихся списка (build → машина → живое); `roots` — только acting layer.

Два слоя отказываются отвечать явно, а не молча: какие операции примет живой server, решается per call по его capabilities, а freshness диагностики принадлежит запросу, который её получил. Оба репортят словарь в Configured и `unknown(source)` с причиной в Effective — новый хелпер рядом с `notApplicable`.

Секреты: через шов не проходит ни argv, ни env, ни initialization option, ни settings, ни резолвнутый путь бинаря, ни текст ошибки старта. Manager теперь пишет `startResult{attempted, kind ErrorKind}` рядом с уже существующим `lastStartErr` — сообщение может назвать резолвнутый бинарь или процитировать вывод сервера, категория не может. `LSPInstallUnknown` для пустой команды: `lsp.json` не читается в обход его же trust rules.

`integrations.go` разделён на общую половину и по файлу на сервис (`integrations_mcp.go`, `integrations_lsp.go`) — второй сервис иначе делает один файл нечитаемым.

**Тесты.** `internal/lsp/observe_test.go` (13): fake-server history не растёт после наблюдения, install lookup не запускает кандидата (маркер-файл), таблица всех `ErrorKind` → категория, JSON-sentinel на конфиг с секретом, закрытый manager, revision, race при shutdown. `internal/diag/integrations_lsp_test.go` (11) + переписанный category-level `integrations_test.go` на оба сервиса. `internal/tui/controller/developer_mode_lsp_test.go` и `cmd/run_developer_lsp_test.go` (по образцу тикета 09) — sentinel-каталог с фейковым gopls в PATH доказывает, что резолвнутый путь никуда не попадает.

**Gates.** gofmt, `golangci-lint run` по четырём пакетам (остался только известный baseline main), `go test` по четырём пакетам, `-race` по `internal/lsp` и `internal/diag`.

## Acceptance Criteria

- integrations.lsp показывает configured/installed/running, workspace, поддерживаемые операции и безопасный source/state.
- Не вызываются server start, download, workspace sync, Query или diagnostics ради самого снимка.
- Сохраняются trust/validation rules LSP config; developer mode не загружает файл обходным способом.
- Неподдержанный язык, выключенный manager, закрытие и неизвестная freshness представлены явно.
- DTO не раскрывает secret-bearing args/env/raw process errors; snapshots detached и безопасны при смене состояния.

## Verification Plan

1. Fixture manager: absent/configured/installed/running/closed/unsupported, корректный workspace.
2. Spies start/download/query/sync не вызываются при harness чтении.
3. Race-проверка затронутого status path при shutdown и tests secret redaction.

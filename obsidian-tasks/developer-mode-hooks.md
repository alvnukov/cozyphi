---
id: developer-mode-hooks
title: 11 — Показать источники и загруженность hooks без выполнения
status: done
priority: medium
model_level: medium
task_type: feature
parent_id: developer-mode-readonly
tags:
    - developer-mode
acceptance_criteria:
    - integrations.hooks показывает known sources, precedence, configured/loaded и безопасные ошибки.
    - Видны тип события и безопасные свойства применения/ограничений без скрипта, env, команды или sensitive arguments.
    - Collector не выполняет hook, не делает rediscovery/reload и не изменяет hook policy.
    - Обычные pre/post hooks самого harness tool call продолжают работать согласно executor; запрет collector execution не отключает tool loop.
    - Отсутствующий manager и stale loaded config явно отличаются от пустого набора hooks.
verification_plan:
    - Fixture global/project precedence, отсутствующий manager и loaded state после изменения config.
    - Collector-only spies доказывают отсутствие hook execution/reload; executor integration сохраняет обычные pre/post hooks.
    - Sentinel scripts/args/env/errors не попадают в harness result/audit.
created_at: "2026-09-06T09:08:35.745834Z"
updated_at: "2026-09-06T17:29:26.123749Z"
---

## Body

**Что построить.** Наблюдение загруженных hooks и их источников через harness integrations. Читать epic developer-mode-readonly.

**Фиксированные решения.** Snapshot строится по данным manager и безопасным metadata загрузчика. Не перечитывать/выполнять скрипты для определения статуса. Не путать наблюдаемый hook со штатным pre/post hook текущего вызова harness: tool-loop invariant остаётся в силе. Source path sanitization обязательна, raw load errors недопустимы.

**Сделано.** `integrations` получила третий сервис — 10 ключей (`hooks.state`, `registered`, `events`, `tools`, `user`, `project`, `blocking`, `async`, `timeout`, `load`), итого 26 ключей в категории. Lifecycle различает `disabled` / `not_loaded` / `load_failed` / `no_manager` / `empty` / `active`: отсутствующий manager, несостоявшийся load и `COZYPHI_HOOKS=off` больше не читаются одинаково как «hooks нет».

`Manager` — это список entries, он не помнит, какая директория определила hook, чьё определение он заменил и сколько проблем load пропустил. Новый `hooks.LoadFacts` (все поля неэкспортируемые) снимает это там, где load происходит, и доносит до места наблюдения: plugin-файлу, run-пути и тексту warning некуда просочиться. `Discovered.Shadowed` записывает вытесненные источники, поэтому precedence заявляется, а не выводится из невидимого merge. Manager и его LoadFacts публикуются парой и меняются вместе (`storeHooks` пишет facts до manager), так что читатель не соберёт manager с чужой записью о load; child наследует запись родителя, а не своего workspace.

Наблюдение не запускает hook, не перечитывает директории, не переразбирает манифест, не пересобирает manager и не меняет политику. Наружу выходят имя, событие, origin, tool-селектор, timeout и два флага; исход load — категория и счётчик пропущенного, но никогда текст проблемы.

**Замер бюджета (для developer-mode-overview-budget).** Detail-ответ категории `integrations` с тремя сервисами: MCP 4811 байт (8 полей), LSP 5459 (8), hooks 5920 (10) — суммарно 16674 против `DefaultMaxTotalBytes = 16384`, из-за чего `hooks.load` молча выпадал. В рамках тикета ужаты только формулировки, написанные здесь же (source refs hooks); MCP/LSP и общий лимит не тронуты. Итог 16134/16384 — 250 байт запаса. В `integrations_test.go` добавлен явный `assert.False(t, snapshot.Truncated)`, чтобы следующий, кто добавит поле, увидел это сразу, а не по пропавшему ключу. Четвёртый сервис в этой категории без работы над бюджетом не поместится.

**Тесты.** `internal/hooks/observe_test.go` (9 тестов, реальные исполняемые скрипты с маркером «я запустился», reflection-allowlist на `manifestFacts`), `internal/diag/integrations_hooks_test.go` (11 тестов на lifecycle, precedence, blocking/async, timeout, detach снапшота, отсутствие места для секрета), `internal/agent/hooks_observe_test.go` (полный `ex.run` с обычным и readonly gate), `internal/tui/controller/developer_mode_hooks_test.go` (4 теста, включая ответ из load до reload), `cmd/run_developer_hooks_test.go` (3 теста в headless-прогоне, каталог заявляет 10 ключей и не предлагает per-hook ключ).

**Gates.** `gofmt -l` по изменённым пакетам чисто; `golangci-lint run` по `internal/hooks internal/diag internal/agent internal/tui/controller cmd` — свои находки (golines ×2, misspell, unparam на `HooksState.field`) исправлены, остался только известный baseline main; `go test` по пяти пакетам зелёный, `-race` на `internal/hooks` и `internal/diag` зелёный.

## Acceptance Criteria

- integrations.hooks показывает known sources, precedence, configured/loaded и безопасные ошибки.
- Видны тип события и безопасные свойства применения/ограничений без скрипта, env, команды или sensitive arguments.
- Collector не выполняет hook, не делает rediscovery/reload и не изменяет hook policy.
- Обычные pre/post hooks самого harness tool call продолжают работать согласно executor; запрет collector execution не отключает tool loop.
- Отсутствующий manager и stale loaded config явно отличаются от пустого набора hooks.

## Verification Plan

1. Fixture global/project precedence, отсутствующий manager и loaded state после изменения config.
2. Collector-only spies доказывают отсутствие hook execution/reload; executor integration сохраняет обычные pre/post hooks.
3. Sentinel scripts/args/env/errors не попадают в harness result/audit.

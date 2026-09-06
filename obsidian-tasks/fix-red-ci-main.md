---
id: fix-red-ci-main
title: Починить красный CI на main
status: done
priority: high
tags:
    - ci
    - lint
    - tests
branch: bug/fix-red-ci-main
worktree_path: .worktrees/fix-red-ci-main
created_at: "2026-09-06T00:00:00Z"
updated_at: "2026-09-06T00:00:00Z"
---

## Body

На GitHub красные все четыре джобы CI (lint, fmt-check, test ubuntu/macos, coverage), а заодно и Release для v0.20.0 — он падает на том же шаге тестов. Последний зелёный прогон — 33979639624 на ea7945b от 2026-09-05.

**Причина.** 2026-09-06 в main влили пачкой несколько веток (d431ae5, 21cbbe3, 87db595, da6ce49, 4f430f1, b4ba5a9, f1960ce) и запушили разом, поэтому CI отработал только на верхушке. Версия golangci-lint (v2.13.0) не менялась — линтерный долг накопился именно из-за непроверенных merge-результатов.

**Три класса поломок и что сделано (2026-09-06, коммит 0b0900c).**

Тесты:

- `internal/tui/statuspane/codex_usage_test.go` ждал строку `manual resets`, которую с 5b0fe1f никто не рендерит: `usagepane` печатает `limit resets`. Правка делегированного ожидания — переименование в 5b0fe1f обновило `usagepane` и `sidebar`, но пропустило `statuspane`.
- `internal/tui/sessions/lifecycle_ownership_test.go` — DATA RACE и `panic: Fail in goroutine after ... has completed`. Проба истории вызывала `require` внутри условия `require.Never`. testify гоняет условие на своей горутине и возвращается, как только вердикт известен, поэтому проба переживала тест и ассертила по снесённой сессии. Теперь проба — чистый предикат, а утверждение о владельце (`require.ErrorIs(..., session.ErrBusy)`) выполняется синхронно на горутине теста.

Линт (16 находок, все закрыты):

- ineffassign в `command_palette.go` — присваивание `visible` перетиралось безусловно ниже; ветка свёрнута в `boxH = max(maxH-2, 4)`.
- modernize `strings.SplitSeq`, perfsprint `errors.New`, testifylint `assert.Empty`, unused `requireRunIdle`, golines в двух файлах.
- unparam: у `text`/`doneText`, `drawWide` и `showToast` убраны параметры, которые всегда получали одно значение.
- usetesting: `t.Context()` там, где вызов в теле теста; `context.WithoutCancel(t.Context())` внутри `t.Cleanup`, где `t.Context()` уже отменён.

**Проверка.** `golangci-lint run ./...` — 0 issues; `golangci-lint fmt --diff ./...` — чисто; `go test -race` по всем затронутым пакетам зелёный, оба чинёных пакета отдельно прогнаны с `-count=3`.

**Осталось за пользователем.** Пуш не делался. После пуша прогон Release для v0.20.0 надо перезапустить (или перетегировать) — он падал на том же шаге тестов.

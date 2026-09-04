---
id: fix-lsp-method-symbol-resolve
title: 'lsp: symbol-резолв не понимает имена методов gopls ((*Recv).Method)'
status: done
priority: high
task_type: bug
tags:
    - bug
    - lsp
branch: bug/fix-lsp-method-symbol-resolve
worktree_path: .worktrees/fix-lsp-method-symbol-resolve
acceptance_criteria:
    - 'symbol=SetStepModel + file=controller.go: calls/hover/definition/references резолвятся в метод, а не в комментарий (identifier not found исчезает)'
    - Квалификатор Controller.SetStepModel совпадает с обоими написаниями gopls ((*Recv).M и Recv.M)
    - Bare-имя метода при нескольких одноимённых методах даёт ErrAmbiguous с рабочим советом квалифицировать
    - go test ./internal/lsp/ зелёный + integration-тест на реальном gopls пинит резолв метода
verification_plan:
    - go test ./internal/lsp/
    - go test -tags integration -run TestRealGoplsSmoke ./internal/lsp/
    - make fmt-check lint
    - ручная проверка через lsp-инструмент после фикса
created_at: "2026-09-04T20:09:04.888549Z"
updated_at: "2026-09-04T21:10:00.213739Z"
---

## Body

**Bug.** `lsp calls/hover/definition/references` с `symbol` = имени метода падают или
возвращают пустой результат; file-less запрос тоже не находит метод.

**Diagnosis (2026-09-04):** gopls с `hierarchicalDocumentSymbolSupport: true`
возвращает Go-методы как top-level DocumentSymbol с полным именем
`(*Controller).SetStepModel` и пустым контейнером; `workspace/symbol` пишет
`Controller.SetStepModel`. `symbolMatches` (internal/lsp/symbol.go) сравнивает
только `fs.name == symbol` и `fs.container+"."+fs.name == symbol` — оба
промаха. Дальше `occurrenceTarget` берёт первое вхождение имени в тексте файла,
а это doc-комментарий над методом: hover/definition пустые, references/calls
получают от gopls "no identifier found" / "identifier not found".

Пример: `lsp calls internal/tui/controller/controller.go symbol=SetStepModel` →
`rpc error 0: identifier not found`; точная позиция 866:22 работает.
Просто функции (`NewController`, `LoadConfig`) работают — поэтому smoke-тест
на реальном gopls дефект не поймал (пинил только функцию).

**Fix:** в `symbolMatches` принимать receiver-qualified написания — bare-имя
метода и `Recv.Method` для обоих форм; тесты: unit на таблице написаний,
fake-сервер с gopls-формой `(*Recv).Method`, integration-пин метода
(`(*Manager).Query` в internal/lsp/manager.go). CHANGELOG под Unreleased.

**Started (2026-09-04).** Диагноз подтверждён дампом реального ответа gopls: методы приходят top-level с именем (*Controller).SetStepModel, container пуст. Чиню symbolMatches в worktree.

**Note (2026-09-04).** Worktree .worktrees/fix-lsp-method-symbol-resolve создан (база 804db3b). Диагноз подтверждён дампом реального gopls: documentSymbol отдаёт методы top-level с именем (*Controller).SetStepModel и пустым container (selection 865:21), workspace/symbol — Controller.SetStepModel. symbolMatches не матчит ни bare-имя, ни Recv.Method; occurrenceTarget fallback берёт doc-комментарий → identifier not found. Правки: symbolMatches + splitMethodName, тесты, CHANGELOG.

**Done (2026-09-05).** 2026-09-04: fixed. symbolMatches now matches gopls method spellings via splitMethodName ((*Recv).M, Recv.M, (Recv).M, generic [T] suffix trimmed); bare name across several receivers stays a typed ambiguity. Tests: 4 added in internal/lsp/targeting_test.go (proven FAIL without the fix); integration pin (*Manager).Query in smoke_real_test.go green on real gopls (def -> manager.go:84:19, calls 50 edges). Gates: go test ./internal/lsp/ ok, make fmt-check lint 0 issues. Landed on branch bug/fix-lsp-method-symbol-resolve (worktree .worktrees/fix-lsp-method-symbol-resolve): e6897b7 fix + 0a0eb45 style (pre-existing revive unused-parameter in /effort completer, base 804db3b, broken on main too). CHANGELOG Unreleased entry added.

## Acceptance Criteria

- symbol=SetStepModel + file=controller.go: calls/hover/definition/references резолвятся в метод, а не в комментарий (identifier not found исчезает)
- Квалификатор Controller.SetStepModel совпадает с обоими написаниями gopls ((*Recv).M и Recv.M)
- Bare-имя метода при нескольких одноимённых методах даёт ErrAmbiguous с рабочим советом квалифицировать
- go test ./internal/lsp/ зелёный + integration-тест на реальном gopls пинит резолв метода

## Verification Plan

1. go test ./internal/lsp/
2. go test -tags integration -run TestRealGoplsSmoke ./internal/lsp/
3. make fmt-check lint
4. ручная проверка через lsp-инструмент после фикса

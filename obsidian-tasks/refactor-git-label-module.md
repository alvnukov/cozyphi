---
id: refactor-git-label-module
title: 'Git-метка: один module в pathutil вместо двух механизмов'
status: todo
priority: low
task_type: refactor
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - один механизм git-метки
    - нет git-вызова с UI-потока
    - час инъектируется
verification_plan:
    - go test ./internal/tui/pathutil/...
created_at: "2026-08-23T15:17:22.122108Z"
updated_at: "2026-08-23T15:17:22.122108Z"
---

## Body

editor.go:614-660 — branchWatch/branchState/resolveGitDir: сырой парсинг .git/HEAD + gitdir: внутри editor-пакета (~50 строк); параллельно pathutil.go:27-42 шеллит git branch --show-current с context.Background() (:38), вызываемый с UI-потока (newChatInput, pane.go:623). Одно понятие — два module, два режима отказа. Кандидат: всё в pathutil с одним представлением (HEAD-поллинг) и инъектируемым часом; editor оставляет себе только Publish(BranchLabelMsg).

## Acceptance Criteria

- один механизм git-метки
- нет git-вызова с UI-потока
- час инъектируется

## Verification Plan

1. go test ./internal/tui/pathutil/...

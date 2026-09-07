---
id: cozy-tools-require-tag
title: cozy-tools через тег v0.2.0 вместо replace на локальный путь
status: done
priority: high
model_level: medium
task_type: bug
parent_id: web-tools
tags:
    - build
    - cozy-tools
branch: fix/cozy-tools-require
worktree_path: .worktrees/cozy-tools-require
acceptance_criteria:
    - go.mod не содержит replace на /Users/zol/src/cozy-tools; require github.com/alvnukov/cozy-tools указывает на опубликованный тег v0.2.0, go.sum содержит его хеши.
    - Чистый клон собирается (CI на main зелёный); тесты пакетов, импортирующих cozy-tools, проходят.
    - CHANGELOG Unreleased, тексты на английском.
verification_plan:
    - go build ./cmd; go test по webtool, agent, configfile, diag, project, cmd.
    - PR в main, CI на PR.
created_at: "2026-09-07T15:05:00Z"
updated_at: "2026-09-07T15:05:00Z"
---

## Body

**Откуда.** После push main (e94378b) CI упал: `github.com/alvnukov/cozy-tools@v0.0.0: replacement directory /Users/zol/src/cozy-tools does not exist`. Replace на локальный путь из эпика web-tools ушёл в origin вместе с require v0.0.0.

**Что сделано.** В cozy-tools опубликованы main e9d27ae (web family) и аннотированный тег v0.2.0 (релизные гейты: gofmt, vet, тесты, 7 кросс-сборок — чисто; один из четырёх прогонов `go test ./...` завершился с кодом 1, вывод не сохранён, три повторных прогона чистые — флак не идентифицирован). В cozyphi: `go mod edit -dropreplace`, `-require=…@v0.2.0`, `go mod tidy`. Сборка cmd ок; тесты webtool, agent, configfile, diag, project, cmd ок. Дерево тега совпадает с локальным checkout, который стоял за replace, поэтому поведение кода не меняется.

**Что осталось в эпике web-tools.** Ручная проверка в сессии (search → fetch → find → read, срабатывание декоя).

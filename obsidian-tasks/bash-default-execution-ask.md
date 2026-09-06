---
id: bash-default-execution-ask
title: 'Bash: спрашивать разрешение на тесты и сборку по умолчанию'
status: done
priority: critical
model_level: medium
task_type: bug
parent_id: remove-executable-default-bash-allowlist
tags:
    - security
    - permissions
branch: bug/bash-default-execution-ask
worktree_path: .worktrees/bash-default-execution-ask
acceptance_criteria:
    - Стандартная политика требует Ask для go test, go build, go generate и других команд Go из прежнего общего правила, способных исполнять код или изменять среду; неизвестные формы не получают Allow.
    - Явный пользовательский opt-in продолжает разрешать согласованные команды; существующий Deny не ослаблен.
    - Безопасные разрешения не расширены ради сохранения старых тестов; изменение default-поведения документировано в Unreleased.
    - Регрессионные тесты проверяют публичное решение permission gate без реального исполнения недоверенного кода.
verification_plan:
    - Найти актуальную сборку default policy и её публичный интерфейс через LSP; проверить существующие тесты, не переписывать архитектуру.
    - Добавить table-driven проверки default Ask для test/build/generate, build с toolexec, неопасных контрольных команд, explicit allow и deny.
    - Запустить адресные тесты изменённого пакета; проверить diff и changelog. Не запускать вредоносные payloads и lint без отдельного согласия.
created_at: "2026-09-06T09:24:24.531774Z"
updated_at: "2026-09-06T10:04:43.124757Z"
---

## Body

**Родитель:** remove-executable-default-bash-allowlist.

**Результат:** обычный запуск тестов или сборки агентом требует согласия пользователя, если тот явно не разрешил его своей политикой. Bash allowlist не является песочницей; даже сборка может запускать процессы через параметры инструментов.

**Границы:** минимально сократить стандартные Go auto-allow правила, проверить сохранение пользовательских overrides и deny-приоритета. Не строить общий shell sandbox, не менять MCP trust и механизм подтверждений. Разбор составных команд и привязка согласия — отдельная следующая задача.

**Блокируется:** нет — можно начинать сразу.

**Исполнение:** model_level medium; при делегировании явно effort medium или ниже. Код только в task worktree; реестр через native task в main; MCP не использовать. Проверки адресные; дополнительных lint-прогонов без нового разрешения не делать.

**Done (2026-09-06).** Done 2026-09-06: default bash allowlist trimmed to strictly read-only — `^go version\b` and flagless `^go list( [^-][^ ]*)*$` (build flags like -export run the toolchain); test/build/vet/run/generate/fmt/mod/env all ask. Regression tests in internal/permission/gate_test.go (TestDefaultGoCommandsAskByDefault incl. adversarial pins: go list -export, go env GOOS, git -c diff.external=… → Ask; TestUserAllowOptsIntoGoTest keeps opt-in + deny priority). CHANGELOG Unreleased updated. Landed via 82354f7, merged 6485282.

Out-of-scope findings from spec review → for parent remove-executable-default-bash-allowlist: (1) defaultSensitivePaths has no .git/** entries — writes to .git/config or hooks are in-workspace Allow (AC3 unmet); (2) allowed git diff/show/log honor repo-controlled diff.external/textconv, chaining with (1) into an approval-free RCE path; (3) no test that readonly/plan mode folds go test Ask→Deny (TestModeReadonlyDeniesWrite only covers write + allowlisted git status); (4) compound-command parsing and parameter-bound approval explicitly deferred. Also 4 pre-existing golangci-lint hits outside this diff (controller.go:2363 unused requireRunIdle; lifetime_test.go:14 usetesting t.Context()).

## Acceptance Criteria

- Стандартная политика требует Ask для go test, go build, go generate и других команд Go из прежнего общего правила, способных исполнять код или изменять среду; неизвестные формы не получают Allow.
- Явный пользовательский opt-in продолжает разрешать согласованные команды; существующий Deny не ослаблен.
- Безопасные разрешения не расширены ради сохранения старых тестов; изменение default-поведения документировано в Unreleased.
- Регрессионные тесты проверяют публичное решение permission gate без реального исполнения недоверенного кода.

## Verification Plan

1. Найти актуальную сборку default policy и её публичный интерфейс через LSP; проверить существующие тесты, не переписывать архитектуру.
2. Добавить table-driven проверки default Ask для test/build/generate, build с toolexec, неопасных контрольных команд, explicit allow и deny.
3. Запустить адресные тесты изменённого пакета; проверить diff и changelog. Не запускать вредоносные payloads и lint без отдельного согласия.

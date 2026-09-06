---
id: bash-approval-command-binding
title: 'Bash: привязать разрешение к полной исполняемой команде'
status: done
priority: critical
model_level: medium
task_type: bug
parent_id: remove-executable-default-bash-allowlist
tags:
    - security
    - permissions
branch: bug/bash-approval-command-binding
worktree_path: .worktrees/bash-approval-command-binding
acceptance_criteria:
    - Allow для безопасного префикса не распространяется на исполняющий хвост, pipeline, подстановку или перенаправление с дополнительными эффектами.
    - Пользователь подтверждает полную фактически исполняемую команду и значимый контекст запуска; изменение параметров после подтверждения требует новой проверки.
    - Неподдержанный или неоднозначный синтаксис не получает автоматический Allow; Deny сохраняет приоритет.
    - Adversarial и положительные тесты идут через публичную границу проверки/исполнения; отказ или отмена подтверждения гарантируют отсутствие запуска.
    - Документированы точные пределы поддержанного синтаксиса и добавлена запись Unreleased при изменении поведения.
verification_plan:
    - Через LSP проследить путь запроса от permission gate до запуска; зафиксировать, какие поля входят в подтверждение и где возможна мутация.
    - Добавить таблицу для &&, ||, ;, pipeline, переводов строк, command substitution, кавычек и перенаправлений; включить безопасные контрольные случаи и неизвестный синтаксис.
    - 'Добавить регрессию изменения команды/контекста после Ask и отмены: fake executor не должен запускаться без разрешения точного запроса.'
    - Запустить адресные permission/executor тесты; при конкурентном состоянии — scoped race. Не запускать payloads на машине и lint без согласия.
created_at: "2026-09-06T09:24:51.067396Z"
updated_at: "2026-09-06T11:24:23.685617Z"
---

## Body

**Родитель:** remove-executable-default-bash-allowlist.

**Результат:** после сокращения default allowlist безопасная команда не служит разрешённым префиксом для опасного продолжения, а подтверждение относится к неизменному исполняемому запросу.

**Границы:** проверить существующий shell-разбор, показ Ask и передачу одобренного запроса исполнителю; исправлять только подтверждённые пробелы. Использовать существующий parser, если он есть; не писать полноценный shell и не вводить новые зависимости без обоснования. Не решать sandbox, MCP trust и защиту файлов конфигурации. Не расширять explicit allow неявно.

**Блокируется:** bash-default-execution-ask — тестировать окончательный набор стандартных разрешений. Пока зависимость не закрыта, не начинать.

**Исполнение:** model_level medium; при делегировании явно effort medium или ниже. Код в task worktree, реестр native task в main, без MCP. Адресные проверки; lint только после нового разрешения.

**Reopened (2026-09-06).** Блокер bash-default-execution-ask закрыт 2026-09-06 (82354f7, merge 6485282, леджер 0b83fb1): финальный default-набор разрешений зафиксирован тестами — можно тестировать привязку подтверждения поверх него.

**Done (2026-09-06).** Closed via merge b9dbc18 (branch bug/bash-approval-command-binding, work commit b8bf427, base 4083990). Gate: hasBashControlSyntax rewritten — expansion ($(), ${}, backticks) asks even inside double quotes; chaining (newline, ;, |, ||, &&, background &) and subshell/process-substitution parens ask; input redirects and heredocs (<, <<) ask; unclosed quotes/dangling escape fail closed; >/dev/null exemption matches exactly /dev/null (>> and FD-digit forms allowed, lookalikes ask). bashEligibleForAllowlist doc comment enumerates the exact supported-syntax limits; CHANGELOG Unreleased carries the Security entry. Tests: TestBashAllowlistBindsToTheFullCommand (red-first table incl. \n, ||, <, <<, lookalike /dev/nullx) + executor pins TestBashApprovalBindsToTheCommandThatRuns / TestBashApprovalDoesNotCarryAcrossCommands. Full go test ./... green, gofmt clean. Review: two-axis (Standards/Spec) sequential sub-agents; blocking findings fixed (doc limits, \n/|| pins, '<' control, ${} wording, 2>>/dev/null pin); lone-& claim rejected against 4083990. Timeout-as-context: the command is the bash request's context; deny/cancel no-run covered by existing executor tests.

## Acceptance Criteria

- Allow для безопасного префикса не распространяется на исполняющий хвост, pipeline, подстановку или перенаправление с дополнительными эффектами.
- Пользователь подтверждает полную фактически исполняемую команду и значимый контекст запуска; изменение параметров после подтверждения требует новой проверки.
- Неподдержанный или неоднозначный синтаксис не получает автоматический Allow; Deny сохраняет приоритет.
- Adversarial и положительные тесты идут через публичную границу проверки/исполнения; отказ или отмена подтверждения гарантируют отсутствие запуска.
- Документированы точные пределы поддержанного синтаксиса и добавлена запись Unreleased при изменении поведения.

## Verification Plan

1. Через LSP проследить путь запроса от permission gate до запуска; зафиксировать, какие поля входят в подтверждение и где возможна мутация.
2. Добавить таблицу для &&, ||, ;, pipeline, переводов строк, command substitution, кавычек и перенаправлений; включить безопасные контрольные случаи и неизвестный синтаксис.
3. Добавить регрессию изменения команды/контекста после Ask и отмены: fake executor не должен запускаться без разрешения точного запроса.
4. Запустить адресные permission/executor тесты; при конкурентном состоянии — scoped race. Не запускать payloads на машине и lint без согласия.

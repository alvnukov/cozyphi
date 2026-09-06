---
id: developer-mode-tui
title: 02 — Подключить developer mode к TUI без наследования детьми
status: done
priority: medium
model_level: medium
task_type: feature
parent_id: developer-mode-readonly
tags:
    - developer-mode
acceptance_criteria:
    - cozyphi --developer-mode и cozyphi tui --developer-mode дают тот же harness contract, что headless.
    - Все пользовательские сессии данного developer-процесса получают read-доступ; доступ не включается для дочерних агентов, включая интерактивных и headless детей.
    - Resume developer-сессии в процессе без флага не восстанавливает capability; config/ENV/UI/model rebind не добавляют доступ.
    - Закрытие/переключение сессии не подменяет snapshot чужой сессией и не закрывает заимствованные общие ресурсы.
    - Нет новой UI-кнопки активации, launcher или отдельного developer Ask; обычные permission/plan gates сохранены.
verification_plan:
    - Table tests CLI entrypoints и старт без флага после resume.
    - Integration tests root/new user session/interactive child/headless child и tool rebind.
    - Профильные lifecycle tests на переключение/закрытие; race только для затронутой сборки, без терминала и сети.
created_at: "2026-09-06T09:06:41.965202Z"
updated_at: "2026-09-06T10:20:19.748202Z"
---

## Body

**Что построить.** Подключить готовый read-only путь к TUI и доказать границы стартовых полномочий на lifecycle пользовательских и агентских сессий. Читать общий контракт epic developer-mode-readonly.

**Blocked by:** developer-mode-headless.

**Scope.** Сборка CLI/TUI и привязка snapshot к текущей сессии; не мигрировать весь /status и не вводить новый экран. Полномочия фиксированы на старте процесса и явно передаются только пользовательским sessions. Общие менеджеры process/workspace не являются источником ambient authority для детей. Не сериализовать capability в историю или job metadata как восстанавливаемое разрешение.

**Работа.** Код в task worktree; узкие проверки и closeout по epic. После завершения blocker задачу можно перевести в todo.

## Acceptance Criteria

- cozyphi --developer-mode и cozyphi tui --developer-mode дают тот же harness contract, что headless.
- Все пользовательские сессии данного developer-процесса получают read-доступ; доступ не включается для дочерних агентов, включая интерактивных и headless детей.
- Resume developer-сессии в процессе без флага не восстанавливает capability; config/ENV/UI/model rebind не добавляют доступ.
- Закрытие/переключение сессии не подменяет snapshot чужой сессией и не закрывает заимствованные общие ресурсы.
- Нет новой UI-кнопки активации, launcher или отдельного developer Ask; обычные permission/plan gates сохранены.

## Verification Plan

1. Table tests CLI entrypoints и старт без флага после resume.
2. Integration tests root/new user session/interactive child/headless child и tool rebind.
3. Профильные lifecycle tests на переключение/закрытие; race только для затронутой сборки, без терминала и сети.

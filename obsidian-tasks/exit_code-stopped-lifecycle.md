---
id: exit_code-stopped-lifecycle
title: 'Фоновые задачи: честный exit_code у stopped и безромянный lifecycle-отчёт'
status: done
priority: medium
task_type: bug
tags:
    - shell
    - background-tasks
branch: bug/exit_code-stopped-lifecycle
worktree_path: .worktrees/exit_code-stopped-lifecycle
acceptance_criteria:
    - В терминальной нотификации, shell_task list и shell_task get у задачи в состоянии stopped значение exit_code не читается как успех (null, пропуск или явный маркер) — одинаково на всех трёх поверхностях
    - Поле deadline не появляется в payload, когда таймаут не задан (сейчас утекает 0001-01-01T00:00:00Z)
    - stop уже терминальной задачи отвечает сразу её фактическим состоянием (completed/stopped) и не обещает подтверждение, которого не будет
    - Регрессионные тесты на все три случая (нотификация-формирователь + рендеры list/get)
verification_plan:
    - go build + go test по изменённым пакетам (bashtool/controller — по факту затронутых)
    - Один scoped golangci-lint run по изменённым пакетам перед коммитом
    - 'Живой прогон: фоновая задача → stop → нотификация и list не показывают exit_code:0 у stopped; stop завершённой отвечает сразу'
    - 'CHANGELOG: строка под [Unreleased]'
created_at: "2026-09-13T10:51:22.299184Z"
updated_at: "2026-09-13T11:10:00.083652Z"
---

## Body

**Что:** три косметических дефекта lifecycle-отчёта фоновых shell-задач (#19), найденных живым прогоном 2026-09-13 в сессии тестирования нового функционала.

**Факты (все воспроизведены живыми вызовами):**
1. Остановленная через `shell_task stop` задача приходит в терминальной нотификации с `state:"stopped"`, но `exit_code:0` — процесс убит, а код читается как успех; в `shell_task list` то же самое. Любой код, который смотрит на exit_code, обманут.
2. В payload нотификации утекает zero-value `deadline:"0001-01-01T00:00:00Z"`, когда таймаут не задан.
3. `shell_task stop` по уже завершённой задаче отвечает «Stop requested… Completion will confirm that the process exited», но терминальное событие уже отстрелило — подтверждение не приходит никогда. Не ошибка, но сообщение обещает лишнее.

**Границы:** меняется только отчётность (payload нотификации + рендер list/get + ответ stop); механика запуска/остановки/нотификаций не трогается. Пример live-payload для фиксстуры есть в сессии от 2026-09-13 (задачи sh-IRDN…, sh-7ZBS…, sh-OXX4…).

**Started (2026-09-13).** Взял в работу 2026-09-13 после живого тестирования фоновых задач; работа в worktree/ветке задачи, гейты только по изменённым пакетам, подписанный коммит без push.

**Done (2026-09-13).** Реализовано на ветке bug/exit_code-stopped-lifecycle (worktree .worktrees/exit_code-stopped-lifecycle), подписанный коммит 29625dcc без push. Snapshot получил честные типы: ExitCode *int (nil у running/stopped), Deadline/Finished *time.Time (нет zero-value 0001-01-01 в payload); терминальный статус вынесен в terminalState(); shell_task stop терминальной задачи отвечает «already <state>; no process to stop». Новые регрессии: internal/shelltask/lifecycle_test.go, internal/tools/bashtool/background_lifecycle_test.go; обновлены pane/transcript-тесты. Scoped-гейты: build+test затронутых пакетов зелёные (shelltask, bashtool, shellpane, transcript, controller, agent), один lint-прогон чист после правки 2 usetesting, gofmt стабилен. CHANGELOG + doc/shell-tasks.md обновлены. Далее: PR, review, зелёный CI — потом блокер снят.

## Acceptance Criteria

- В терминальной нотификации, shell_task list и shell_task get у задачи в состоянии stopped значение exit_code не читается как успех (null, пропуск или явный маркер) — одинаково на всех трёх поверхностях
- Поле deadline не появляется в payload, когда таймаут не задан (сейчас утекает 0001-01-01T00:00:00Z)
- stop уже терминальной задачи отвечает сразу её фактическим состоянием (completed/stopped) и не обещает подтверждение, которого не будет
- Регрессионные тесты на все три случая (нотификация-формирователь + рендеры list/get)

## Verification Plan

1. go build + go test по изменённым пакетам (bashtool/controller — по факту затронутых)
2. Один scoped golangci-lint run по изменённым пакетам перед коммитом
3. Живой прогон: фоновая задача → stop → нотификация и list не показывают exit_code:0 у stopped; stop завершённой отвечает сразу
4. CHANGELOG: строка под [Unreleased]

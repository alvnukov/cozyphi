---
id: session-worktree-switch
title: 'Переключатель сессии в ворктри: рабочая область и леджер задач от ворктри'
status: todo
priority: high
model_level: high
task_type: feature
tags:
    - worktree
    - session
    - tasks
    - registry
    - tools
acceptance_criteria:
    - Переключённая в ворктри сессия выполняет bash и файловые тулы от корня ворктри; возврат в main — тем же переключателем
    - Тула task пишет и читает леджер в obsidian-tasks/ активного ворктри; из main-сессий поведение прежнее
    - Permission workspace, проект-инструкции и git-контекст перевязываются на ворктри без открытия побегов; память остаётся общим корпусом
    - Поверхность переключателя зафиксирована в дизайне и задокументирована (doc/, CHANGELOG)
verification_plan:
    - go test по затронутым пакетам (internal/tasks, internal/tools/tasktool, internal/permission, internal/project, internal/agent) -race
    - 'Живой прогон: сессия в main → переключение → task start/note оставляют правки в ворктри, git-статус main чист'
    - Один scoped golangci-lint по изменённым пакетам
    - CHANGELOG под Unreleased + правка doc (project-layout/tasks.md)
created_at: "2026-09-13T19:41:50.399646Z"
updated_at: "2026-09-13T19:51:12.484742Z"
---

## Body

**Проблема.** Процесс требует работу в `.worktrees/<branch>` при main на main, но сессия харнесса, начатая в main-чекауте, работает против main: bash и файловые тулы берут cwd main (модель таскает префикс `.worktrees/...` в каждом вызове), а нативная тула task через discovery по git common dir пишет леджер в реестр главного чекаута — правки заметок висят грязью в main и попадают в PR только ручным переносом (миграционная шероховатость из commit-every-feature).

**Цель.** Переключатель, переводящий активную сессию в ворктри задачи: cwd bash и база относительных путей, target реестра задач (ledger branch-local), git-контекст и границы permission workspace перевязываются на ворктри; память остаётся общим корпусом для всех ворктри (by design); возврат в main — тем же механизмом.

**Открытые вопросы (до дизайна).** Поверхность: автоматика в `task start` (он уже знает ветку и ворктри и печатает команду создания), явная slash-команда, таба мультисессии с cwd ворктри или тулa модели; нужно ли переключение живой сессии или достаточно старта сессии в ворктри + починка target реестра; что перевязывать: project instructions, LSP root, MCP workspace cwd; как переключение проходит через permission gate (пересборка workspace без открытия побегов).

**Связанное.** native-task-tool и task-writes-in-plan-mode (реестр через main), multisession-projects / session-local-shell-ownership (per-session cwd), commit-every-feature (branch-local леджер).

**Note (2026-09-13).** Дизайн-развилка, поднятая пользователем: «задача должна появиться в реестре, а мы в main не мержим» — конфликт «живой общий реестр» vs «main только через PR». Вариант A: реестр как данные — вывести obsidian-tasks/ из git cozyphi (при желании истории — собственный git-репозиторий), заведение задачи видно всем сразу, PR везут только код, скоуп переключателя сжимается до перепривязки рабочей области; явно отменяет решение 2026-09-08 «леджер едет в PR». Вариант B: реестр остаётся в git — бэклог живёт грязью main как staging, start уводит заметку в ветку первым коммитом, возврат в main только мержем PR (задача временно пропадает из живого реестра main). Ждём выбора пользователя; выбор меняет скоуп задачи.

**Note (2026-09-13).** Рескоуп по решению пользователя: v1 — не сессионный переключатель, а явная цель реестра в тулу task (выделено в task-tool-registry-target): main / ворктри / внешний корень, sticky на задачу. Сессионный переключатель рабочей области (cwd bash, файловые тулы, permission workspace, инструкции, LSP) и авто-создание ветки+ворктри на task start — отложены до появления трения. Развилка A/B (реестр в git vs данные вне git cozyphi) остаётся открытой.

## Acceptance Criteria

- Переключённая в ворктри сессия выполняет bash и файловые тулы от корня ворктри; возврат в main — тем же переключателем
- Тула task пишет и читает леджер в obsidian-tasks/ активного ворктри; из main-сессий поведение прежнее
- Permission workspace, проект-инструкции и git-контекст перевязываются на ворктри без открытия побегов; память остаётся общим корпусом
- Поверхность переключателя зафиксирована в дизайне и задокументирована (doc/, CHANGELOG)

## Verification Plan

1. go test по затронутым пакетам (internal/tasks, internal/tools/tasktool, internal/permission, internal/project, internal/agent) -race
2. Живой прогон: сессия в main → переключение → task start/note оставляют правки в ворктри, git-статус main чист
3. Один scoped golangci-lint по изменённым пакетам
4. CHANGELOG под Unreleased + правка doc (project-layout/tasks.md)

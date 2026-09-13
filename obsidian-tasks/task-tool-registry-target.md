---
id: task-tool-registry-target
title: 'Тула task: явная цель реестра (main / ворктри / внешний корень)'
status: done
priority: high
model_level: high
task_type: feature
tags:
    - tasks
    - registry
    - tools
    - worktree
branch: feature/task-tool-registry-target
worktree_path: .worktrees/task-tool-registry-target
acceptance_criteria:
    - Без параметра корень реестра — git-корень папки запуска сессии (main-сессия — реестр main; сессия в ворктри — реестр этого ворктри); старый резолв в главный чекаут удалён
    - 'Параметр-цель выбирает известный корень: главный чекаут, живые ворктри, внешние из конфига; неизвестное значение — ошибка со списком известных'
    - Мутации пишут в реестр выбранного корня; каждый ответ называет абсолютный путь изменённого файла; вызов без параметра — всегда дефолт, без скрытой памяти целей
    - 'Формат заметок байт-совместим с mcp-ai-helper; gate: task_read/task_write по permissions.tasks, записи не выходят за заверенные корни'
    - Промпт-абзац и doc/tasks.md описывают дефолт и известные корни; CHANGELOG под Unreleased
verification_plan:
    - go test ./internal/tasks/... ./internal/tools/tasktool/... ./internal/permission/... ./internal/agent/... -race
    - 'Живой прогон из ворктри: без параметра — реестр ворктри; с целью main — полный бэклог; мутации оставляют правки в выбранном корне, git-статус второго корня чист'
    - 'Совместимость: заметка из внешнего корня читается mcp-ai-helper task get без диагностик'
    - Один scoped golangci-lint по изменённым пакетам
created_at: "2026-09-13T19:51:12.481881Z"
updated_at: "2026-09-13T22:10:15.108735Z"
---

## Body

**Проблема.** Нативная тула task пишет леджер всегда в реестр главного чекаута (discovery через RepoRoot → git common dir). При работе в `.worktrees/<branch>` правки заметок висят грязью в main и в PR попадают только ручным переносом; реестр жёстко привязан к репозиторию.

**Решение (согласовано с пользователем).** Дефолт: корень реестра — git-корень папки запуска сессии (`git rev-parse --show-toplevel` от cwd запуска, новый хелпер рядом с claudeMemoryRoot); резолв «всегда в главный чекаут» убирается из discovery тулы. Если у корня запуска реестра нет, а отличный от него main-чекаут его имеет — дефолт main (сохраняет гарантию «ворктри-сессия всегда видит реестр»). Параметр-цель `root`: при записи (create/update/start/done/block/reopen/note) обязателен — модель явно называет папку реестра, куда писать; при чтении (current/list/get) необязателен, по умолчанию корень запуска, папкой можно посмотреть другой реестр. Известные значения: `main` (главный чекаут), ярлык живого ворктри из `git worktree list --porcelain` (базовое имя каталога, для задачных деревьев — id задачи; резолвится на каждый вызов — свежий ворктри доступен без перезапуска), имя внешнего корня из глобального конфига (`tasks.roots` в ~/.cozyphi/config.yaml, заверяет человек — реестр хоть в другой репе). Неизвестная цель — ошибка со списком известных и текущим ярлыком. Описание тулы понятно объясняет моделью семантику «запись называет папку, чтение — по умолчанию»; current и тексты ошибок перечисляют живые корни. Ответы называют путь файла (displayPath уже даёт абсолютный вне cwd). Никакой скрытой памяти целей: вызов без параметра — снова дефолт.

**Форма (по разведке).** internal/project: `CheckoutRoot()` через --show-toplevel, RepoRoot остаётся для памяти. internal/tasks: `DiscoverTargets(launchRoot, mainRoot, extra)` → Targets{Default, Resolve(name)}; cmd/run.go:320 и tui/controller/runtime.go:294 переходят на него. internal/tools/tasktool: Tool(targets, access), параметр `root`, обязательность для записей в parse, валидация цели. internal/permission: extract кладёт цель в Target при нестандартном корне (`<root>:<id>`), gate-семантика не меняется: записи не выходят за `<заверенный корень>/obsidian-tasks/<id>.md`. Тесты: project (toplevel из ворктри/подкаталога), tasks (targets, fallback, unknown), tasktool (обязательность root для записей, чтения с целью, абсолютные пути, перечисление корней), permission (Target с целью), engine_task (регистрация).

**Границы.** Sticky-цель на задачу исключена (предсказуемость). Не решает: видимость грязи main для свежего ворктри (развилка A/B «реестр в git vs данные» — открыта, не блокирует); cwd сессии и переключатель рабочей области — отложено в [[session-worktree-switch]].

**Связанное.** session-worktree-switch (отложен), native-task-tool (текущий дизайн), commit-every-feature (branch-local леджер).

**Started (2026-09-14).** в .worktrees/task-tool-registry-target на feature/task-tool-registry-target

**Реализовано (2026-09-14).** internal/tasks/targets.go: DiscoverTargets/Targets (дефолт — корень запуска, fallback main; main + живые ворктри из git worktree list --porcelain без prunable, ярлык — базовое имя каталога; внешние корни из tasks.roots, относительный путь или кража «main» — ошибка конфига); tasktool: Tool(targets, access), параметр root, запись без root — отказ со списком известных и текущим ярлыком, чтения — по умолчанию корень запуска, снимок целей на каждом вызове (ворктри, рождённый в середине сессии, доступен следующим вызовом); project: CheckoutRoot() через --show-toplevel + tasksFileConfig.Roots; permission: root едет в Target как «root/id» только для надписи в утверждении, семантика gate не изменилась; промпт-абзац и doc/tasks.md переписаны; CHANGELOG под Unreleased. Тесты: tasks/targets_test, tasktool (обязательность root, цели, абсолютные пути), project (toplevel из ворктри/подкаталога, tasks.roots), permission, engine_task, developer-mode coverage. Гейты: go test -race по изменённым пакетам зелёный, один scoped golangci-lint — 0 issues.

**Note (2026-09-14).** Живой прогон из main-чекаута на бинараре из ветки: дефолт без root — реестр main (450 задач); root=task-tool-registry-target — реестр ворктри (449, задача in_progress из веточной копии); запись без root и неизвестный root — отказ со списком известных и «this session started in main». Ветка main осталась чистой.

**Done (2026-09-14).** Слит в main как cc44a30b (PR #24, squash, CI зелёный по всем 9 чекам). Реализовано: DiscoverTargets/Targets в internal/tasks (дефолт — корень запуска с fallback на main; main, живые ворктри по базовому имени каталога, внешние корни из tasks.roots; снимок на каждый вызов), параметр root в туле task с обязательностью для записей и отказом со списком известных, CheckoutRoot/Tasks.Roots в internal/project, root/id в Target утверждения пермишена при неизменной gate-семантике, промпт-абзац, doc/tasks.md, CHANGELOG. Живая проверка прогнана на бинараре из ветки (дефолт/цель/отказы/изоляция main), леджерная заметка о ней — в коммите 6e5f2468.

## Acceptance Criteria

- Без параметра корень реестра — git-корень папки запуска сессии (main-сессия — реестр main; сессия в ворктри — реестр этого ворктри); старый резолв в главный чекаут удалён
- Параметр-цель выбирает известный корень: главный чекаут, живые ворктри, внешние из конфига; неизвестное значение — ошибка со списком известных
- Мутации пишут в реестр выбранного корня; каждый ответ называет абсолютный путь изменённого файла; вызов без параметра — всегда дефолт, без скрытой памяти целей
- Формат заметок байт-совместим с mcp-ai-helper; gate: task_read/task_write по permissions.tasks, записи не выходят за заверенные корни
- Промпт-абзац и doc/tasks.md описывают дефолт и известные корни; CHANGELOG под Unreleased

## Verification Plan

1. go test ./internal/tasks/... ./internal/tools/tasktool/... ./internal/permission/... ./internal/agent/... -race
2. Живой прогон из ворктри: без параметра — реестр ворктри; с целью main — полный бэклог; мутации оставляют правки в выбранном корне, git-статус второго корня чист
3. Совместимость: заметка из внешнего корня читается mcp-ai-helper task get без диагностик
4. Один scoped golangci-lint по изменённым пакетам

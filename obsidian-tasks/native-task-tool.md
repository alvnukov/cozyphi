---
id: native-task-tool
title: 'Нативная тула task: реестр задач mcp-ai-helper как естественный инструмент модели'
status: in_progress
priority: high
model_level: high
task_type: feature
tags:
    - tools
    - tasks
    - registry
    - permission
    - prompt
acceptance_criteria:
    - Тула task зарегистрирована в сессии, где найден obsidian-реестр главного чекаута, и отсутствует у sub-agents и в репозитории без реестра; system prompt содержит строку про task только при регистрации.
    - 'Формат заметок байт-в-байт совместим с mcp-ai-helper: заметка, записанная cozyphi, читается хелпером без диагностик, и наоборот; битые заметки пропускаются с диагностикой, а не роняют вызов.'
    - current/get/list/create/update/start/done/block/reopen работают из ворктри против реестра main; каждый ответ содержит строку Next; мутации называют изменённый файл.
    - 'Gate: чтения Allow, записи проходят проверку sensitive paths и запрещены в readonly/plan; body с ''## '' заголовком отвергается с объяснением.'
    - make fmt-check lint test rc=0; CHANGELOG и doc/tasks.md обновлены.
verification_plan:
    - go test ./internal/tasks/... ./internal/tools/tasktool/... ./internal/permission/... ./internal/agent/... -race
    - 'Совместимость: заметка, созданная тулой в temp-реестре, читается mcp-ai-helper task action=get без diagnostics'
    - 'Ручной прогон в TUI из ворктри: task current → start → done; проверка строки Next и пути файла'
    - make fmt-check lint test
created_at: "2026-09-02T22:52:42.806862Z"
updated_at: "2026-09-02T22:52:42.806862Z"
---

## Body

**Цель.** Модель в cozyphi работает с реестром задач (заметки obsidian-tasks/ в формате mcp-ai-helper) через одну нативную тулу `task`, без MCP-обвязки, без repo_path в каждом вызове и без ручных правок файлов. Работа с задачами должна быть естественной: «что брать» → «взять» → «закрыть», с подсказкой следующего шага в каждом ответе.

**Пакет internal/tasks.** Реестр: поиск корня через главный чекаут (git common dir, как у памяти), чтение `.mcp-ai-helper.yaml` (`task_registry.backend: obsidian`, `obsidian.path`, по умолчанию obsidian-tasks), скан заметок с пропуском битых и диагностикой, чтение/запись заметки в формате хелпера (frontmatter yaml.v3 + секции Body / Acceptance Criteria / Verification Plan, RFC3339Nano UTC), нормализация id, вывод ветки `<type>/<id>` и ворктри `.worktrees/<id>`, ранжирование исполнимых задач (in_progress → todo, приоритет, updated_at, id; эпики и blocked — контекст). Из ворктри тула видит реестр main — гоча «реестр в ворктри пустой» исчезает.

**Тула internal/tools/tasktool, имя task.** Действия: current (по умолчанию), get, list (фильтры status/priority/task_type/parent_id/tag/query), create, update, start, done, block, reopen. Ответы — компактный текст, не JSON; каждый заканчивается строкой Next с естественным следующим действием. start сообщает ветку и ворктри и команду `git worktree add`, если ворктри нет; done/block/reopen дописывают в body датированный абзац с note/reason. Тело с `## ` заголовками отвергается с объяснением (формат заметки резервирует их). Все записи называют изменённый файл, чтобы леджер можно было закоммитить на main.

**Permission.** ActionTaskRead — Allow; ActionTaskWrite — путь заметки через проверку sensitive paths, isMutating → deny в readonly/plan. Workspace-only не применяется: реестр по определению живёт в главном чекауте, а тула пишет только `<registry>/<safe id>.md`.

**Регистрация.** EngineOpts.Tasks; тула регистрируется только когда реестр найден (как lsp); sub-agents её не получают; в system prompt одна условная строка. Подключение в cmd/run.go и tui/controller через Project.RepoRoot().

**Документация.** doc/tasks.md, строки в doc/project-layout.md, CHANGELOG.

## Acceptance Criteria

- Тула task зарегистрирована в сессии, где найден obsidian-реестр главного чекаута, и отсутствует у sub-agents и в репозитории без реестра; system prompt содержит строку про task только при регистрации.
- Формат заметок байт-в-байт совместим с mcp-ai-helper: заметка, записанная cozyphi, читается хелпером без диагностик, и наоборот; битые заметки пропускаются с диагностикой, а не роняют вызов.
- current/get/list/create/update/start/done/block/reopen работают из ворктри против реестра main; каждый ответ содержит строку Next; мутации называют изменённый файл.
- Gate: чтения Allow, записи проходят проверку sensitive paths и запрещены в readonly/plan; body с '## ' заголовком отвергается с объяснением.
- make fmt-check lint test rc=0; CHANGELOG и doc/tasks.md обновлены.

## Verification Plan

1. go test ./internal/tasks/... ./internal/tools/tasktool/... ./internal/permission/... ./internal/agent/... -race
2. Совместимость: заметка, созданная тулой в temp-реестре, читается mcp-ai-helper task action=get без diagnostics
3. Ручной прогон в TUI из ворктри: task current → start → done; проверка строки Next и пути файла
4. make fmt-check lint test

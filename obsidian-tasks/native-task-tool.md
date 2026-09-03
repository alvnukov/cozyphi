---
id: native-task-tool
title: 'Нативная тула task: реестр задач mcp-ai-helper как естественный инструмент модели'
status: done
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
updated_at: "2026-09-02T23:36:48.458213Z"
---

## Body

**Цель.** Модель в cozyphi работает с реестром задач (заметки obsidian-tasks/ в формате mcp-ai-helper) через одну нативную тулу `task`, без MCP-обвязки, без repo_path в каждом вызове и без ручных правок файлов. Работа с задачами должна быть естественной: «что брать» → «взять» → «закрыть», с подсказкой следующего шага в каждом ответе.

**Пакет internal/tasks.** Реестр: поиск корня через главный чекаут (git common dir, как у памяти), чтение `.mcp-ai-helper.yaml` (`task_registry.backend: obsidian`, `obsidian.path`, по умолчанию obsidian-tasks), скан заметок с пропуском битых и диагностикой, чтение/запись заметки в формате хелпера (frontmatter yaml.v3 + секции Body / Acceptance Criteria / Verification Plan, RFC3339Nano UTC), нормализация id, вывод ветки `<type>/<id>` и ворктри `.worktrees/<id>`, ранжирование исполнимых задач (in_progress → todo, приоритет, updated_at, id; эпики и blocked — контекст). Из ворктри тула видит реестр main — гоча «реестр в ворктри пустой» исчезает.

**Тула internal/tools/tasktool, имя task.** Действия: current (по умолчанию), get, list (фильтры status/type/tag/parent), create, update, start, done, block, reopen, note. Ответы — компактный текст, не JSON; каждый заканчивается строкой Next с естественным следующим действием. start сообщает ветку и ворктри и команду `git worktree add`, если ворктри нет; done/block/reopen/note дописывают в body датированный абзац. Тело с `## ` заголовками отвергается с объяснением (формат заметки резервирует их). Все записи называют изменённый файл, чтобы леджер можно было закоммитить на main.

**Permission.** ActionTaskRead — Allow; ActionTaskWrite — мутация, isMutating → deny в readonly/plan. Путь проверять нечего: каталог реестра фиксирован на старте, нормализованный id не выходит за `<реестр>/<id>.md`.

**Регистрация.** EngineOpts.Tasks; тула регистрируется только когда реестр найден (как lsp); sub-agents её не получают; в system prompt один условный абзац. Подключение в cmd/run.go и tui/controller через Project.RepoRoot().

**Документация.** doc/tasks.md, строка в doc/project-layout.md, AGENTS.md, CHANGELOG.

**Сделано (2026-09-03).** Смержено в main коммитом 8ca0557 (фича a350be3 на ветке feature/native-task-tool, перед слиянием подтянут main — конфликт только в CHANGELOG). Реализовано всё из описания плюс действие note. Отклонение от AC 4: отдельной проверки sensitive paths для записей нет и она не нужна — каталог реестра фиксирован при обнаружении (путь из конфига не может выйти за корень), id нормализуется, так что запись всегда попадает в `<реестр>/<id>.md`; gate различает task_read (Allow) и task_write (мутация, deny в readonly и plan), покрыто gate_task_test. Совместимость проверена перекрёстно: заметка, записанная internal/tasks, прочитана хелпером через task get без диагностик; на живом реестре cozyphi хелпер и cozyphi одинаково помечают 14 давно битых заметок (в объём не входили), cozy-tools читается без диагностик. Ручной прогон в TUI (VP 3) заменён тестами: engine_task_test (регистрация тулы и абзац промпта только при реестре), тесты tasktool (строка Next и путь файла в каждом ответе), тест project (RepoRoot из линкованного ворктри указывает на главный чекаут). Gate после слияния: make fmt-check lint test rc=0, 92 пакета.

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

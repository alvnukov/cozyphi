---
id: multisession-projects
title: 'Сессии в разных проектах: новая сессия в другом каталоге, недавние проекты, аудит cwd в tools/watches/hooks'
status: todo
priority: high
model_level: high
task_type: feature
parent_id: multisession-mode
tags:
    - cozyphi
    - tui
    - project
    - multisession
branch: feature/multisession-projects
worktree_path: .worktrees/multisession-projects
acceptance_criteria:
    - Новая сессия в другом каталоге создаётся из панели, палитры и /new <path>; недавние проекты предлагаются; несуществующий путь отвергается
    - 'Две сессии в разных проектах работают одновременно: bash/файловые tools, watches, hooks, memory, tasks, LSP/MCP статусы — каждая в своём cwd (тест)'
    - cozyphi sessions list --all показывает сессии всех проектов с путём и заголовком
    - make fmt-check lint test в worktree зелёные
verification_plan:
    - go test -race ./internal/session/... ./internal/tui/sessions/... ./internal/tools/... ./internal/tui/controller/... в worktree
    - 'Живой smoke: cozyphi из ~/src/cozyphi, N → ~/src/другой-проект, в обеих сессиях `pwd` через bash и чтение файла; MCP/LSP статусы в сайдбаре меняются при переключении'
    - golangci-lint run на изменённых пакетах один раз перед коммитом
created_at: "2026-09-04T07:31:55.432981Z"
updated_at: "2026-09-04T07:31:55.432981Z"
---

## Body

**Контекст:** сессии на диске лежат в `~/.cozyphi/session/<ProjectDirName(cwd)>/` (internal/project/session_dir.go), реальный cwd — в заголовке каждого файла. Workspace на cwd — multisession-runtime-split. Процесс запущен из одного cwd, но панель группирует сессии по проектам.

**Что сделать:**
1. `session.RecentProjects(baseDir)`: список каталогов проектов (реальный Cwd из самого свежего заголовка в каждом project-dir, отсортировано по mtime), с фильтрацией несуществующих путей.
2. Ввод пути для «новая сессия в каталоге» (панель `N`, палитра «Новая сессия в проекте…», `/new <path>`): overlay с полем пути (~ и относительные от cwd процесса разворачиваются) и списком недавних проектов с fuzzy-фильтром; несуществующий каталог — ошибка в поле.
3. `Registry.New(cwd)` создаёт Workspace через Runtime (или переиспользует), Controller с `SessionOpts{Cwd, SessionDir: ProjectSessionDir(base, cwd)}`; открытие закрытой сессии другого проекта из «Недавних» и из `/sessions`-пикера — тем же путём.
4. Аудит cwd: все tools (bash, read/write/edit, ls/find/grep, lsp, watch, memory, tasks), hooks и branch-watch берут cwd из engine/session, а не `os.Getwd()`; найденные места исправить и закрыть тестом «две сессии с разными cwd видят разные файлы».
5. Правый сайдбар показывает MCP/LSP статусы Workspace активной сессии; предупреждение `cwdWarning` из Resume теперь не нужно для другого проекта — сессия открывается в своём Workspace.
6. `cozyphi sessions list --all` — по всем проектам с колонкой пути; doc; CHANGELOG.

**Blocked by:** multisession-registry, multisession-sessions-panel, multisession-hotkeys

## Acceptance Criteria

- Новая сессия в другом каталоге создаётся из панели, палитры и /new <path>; недавние проекты предлагаются; несуществующий путь отвергается
- Две сессии в разных проектах работают одновременно: bash/файловые tools, watches, hooks, memory, tasks, LSP/MCP статусы — каждая в своём cwd (тест)
- cozyphi sessions list --all показывает сессии всех проектов с путём и заголовком
- make fmt-check lint test в worktree зелёные

## Verification Plan

1. go test -race ./internal/session/... ./internal/tui/sessions/... ./internal/tools/... ./internal/tui/controller/... в worktree
2. Живой smoke: cozyphi из ~/src/cozyphi, N → ~/src/другой-проект, в обеих сессиях `pwd` через bash и чтение файла; MCP/LSP статусы в сайдбаре меняются при переключении
3. golangci-lint run на изменённых пакетах один раз перед коммитом

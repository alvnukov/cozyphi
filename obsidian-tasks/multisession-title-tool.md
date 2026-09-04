---
id: multisession-title-tool
title: 'Tool `session` для модели: set_title с закреплением пользовательского заголовка и подсказкой в системном промпте'
status: todo
priority: high
model_level: high
task_type: feature
parent_id: multisession-mode
tags:
    - cozyphi
    - tools
    - multisession
branch: feature/multisession-title-tool
worktree_path: .worktrees/multisession-title-tool
acceptance_criteria:
    - Модель может вызвать session set_title; заголовок попадает в jsonl с source=model и виден в футере/списках
    - После /rename вызов set_title отвергается с текстом про закрепление, запись не создаётся
    - Tool не выдаётся sub-agent'ам; системный промпт содержит инструкцию по именованию
    - Тесты sessiontool (валидация, pinned, успех) и рендера строки в транскрипте; make fmt-check lint test в worktree зелёные
verification_plan:
    - go test ./internal/tools/sessiontool/... ./internal/agent/... ./internal/tui/transcript/... в worktree
    - 'Живой smoke: новая сессия, первый промпт — модель называет сессию в первом ходу; /rename, повторная просьба переименовать — модель получает отказ и сообщает об этом'
    - golangci-lint run на изменённых пакетах один раз перед коммитом
created_at: "2026-09-04T07:31:55.42522Z"
updated_at: "2026-09-04T07:31:55.42522Z"
---

## Body

**Контекст:** заголовок сессии хранится в jsonl (multisession-title-entry). Нужна ручка для модели — по образцу `internal/tools/contexttool` (действия через поле `action`) и `plantool`.

**Что сделать:**
1. Пакет `internal/tools/sessiontool`: tool `session`, `action: set_title` с `title`. Валидация как в Manager.SetTitle; ответ содержит итоговый заголовок. Если заголовок закреплён пользователем (source=user) — отказ `title is pinned by the user; ask the user to run /rename` без записи. Описание tool объясняет, когда звать: как только цель сессии ясна (обычно в первом ходу) и при существенной смене темы; заголовок — 3–7 слов на языке пользователя, без кавычек и точки в конце.
2. Регистрация в `internal/agent` (список tools, `EngineOpts`), доступно только главной сессии, sub-agent'ам (agenttool jobs) tool не выдаётся.
3. Системный промпт: короткий абзац про именование сессии через `session set_title`.
4. Вызов set_title в транскрипте рендерится компактно одной строкой «Заголовок: …» (как context/plan строки), а не блоком tool call; тост «Сессия названа: …».
5. Записи в doc/tools.md (или актуальный список tool'ов) и CHANGELOG.

**Границы:** без авто-вызова отдельной модели; без UI панели.

**Blocked by:** multisession-title-entry

## Acceptance Criteria

- Модель может вызвать session set_title; заголовок попадает в jsonl с source=model и виден в футере/списках
- После /rename вызов set_title отвергается с текстом про закрепление, запись не создаётся
- Tool не выдаётся sub-agent'ам; системный промпт содержит инструкцию по именованию
- Тесты sessiontool (валидация, pinned, успех) и рендера строки в транскрипте; make fmt-check lint test в worktree зелёные

## Verification Plan

1. go test ./internal/tools/sessiontool/... ./internal/agent/... ./internal/tui/transcript/... в worktree
2. Живой smoke: новая сессия, первый промпт — модель называет сессию в первом ходу; /rename, повторная просьба переименовать — модель получает отказ и сообщает об этом
3. golangci-lint run на изменённых пакетах один раз перед коммитом

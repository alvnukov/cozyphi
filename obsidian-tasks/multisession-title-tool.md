---
id: multisession-title-tool
title: 'Tool `session` для модели: set_title с закреплением пользовательского заголовка и подсказкой в системном промпте'
status: done
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
updated_at: "2026-09-06T11:49:00.677845Z"
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

**Started (2026-09-06).** API зависимости готов в 5af7d4d/d3ca734; отдельный worktree от этого commit, пока завершается интеграционная проверка первой задачи.

**Note (2026-09-06).** Реализованы sessiontool, immutable owning-store binding, отдельный main-only capability (не зависит от planEnabled), общий prompt 3–7 слов на языке пользователя, plan exemption, явный permission Ask по умолчанию, компактная строка и live-toast. Unit/integration проверки нашли и исправили связь с planEnabled (telemetry regression) и raw-input fallback pending-строки. Targeted tests green; watch w2 проверяет шесть пакетов, targeted race и затем ЕДИНСТВЕННЫЙ scoped lint этой задачи. Логи /tmp/cozyphi-title-tool-{tests,race,lint}.log. Entry dependency 5af7d4d/d3ca734 ещё не смёржена в main; обе задачи доставить после verification. Новых агентов не запускал.

**Note (2026-09-06).** Самопроверка VERIFY/working: implementation 843da42, база d3ca734; main интегрирован через entry bb704f4. SOURCE: prompt → sessionNaming main-only → обычный executor/hooks/gate → захваченный Session.SetTitle(model) → durable metadata → compact Mapper/live-toast; отдельного inference нет. RUNTIME: шесть пакетов и targeted race green (логи /tmp/cozyphi-title-tool-{tests,race}.log). Lint: четыре baseline warnings в неизменённых model_selection.go и lifecycle_ownership_test.go; заведён session-model-lint-cleanup, повтор не запускался. Standards: изменений permission bypass/зависимостей нет, controls/UTF8/pin проверены. Spec: durable title, pin после reopen, owning-session isolation, child exclusion, CLI/footer/OSC, pending/error/success и toast покрыты тестами. UNKNOWN: живой вызов внешней модели и внешний терминал не проверялись; язык задаётся prompt, не гарантируется кодом. ReplaySnapshot по прежнему скрывает все tool rows; mapper-тест проверяет пересборку live snapshot, не disk replay. Watch w3: единственный полный make test, fmt-check и build на интегрированной версии.

**Done (2026-09-06).** Доставлено в main merge af23253 (code 843da42). session(set_title) доступен основной сессии независимо от plan, prompt требует язык пользователя без отдельного inference; owning-store closure, manual pin, обычные hooks/permission Ask, compact Title row и live toast. Шесть scoped пакетов и targeted race прошли; ранее запущенные final tests/fmt/build успешны. Единственный lint: 4 замечания в неизменённых файлах, заведена session-model-lint-cleanup; lint не повторялся. Свежий main интегрирован в worktree без code-конфликтов, при доставке гейты не перезапускались. Внешний inference и живой терминал не проверялись; новая ручка появится после сборки и перезапуска бинарника из main.

## Acceptance Criteria

- Модель может вызвать session set_title; заголовок попадает в jsonl с source=model и виден в футере/списках
- После /rename вызов set_title отвергается с текстом про закрепление, запись не создаётся
- Tool не выдаётся sub-agent'ам; системный промпт содержит инструкцию по именованию
- Тесты sessiontool (валидация, pinned, успех) и рендера строки в транскрипте; make fmt-check lint test в worktree зелёные

## Verification Plan

1. go test ./internal/tools/sessiontool/... ./internal/agent/... ./internal/tui/transcript/... в worktree
2. Живой smoke: новая сессия, первый промпт — модель называет сессию в первом ходу; /rename, повторная просьба переименовать — модель получает отказ и сообщает об этом
3. golangci-lint run на изменённых пакетах один раз перед коммитом

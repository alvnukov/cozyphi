---
id: developer-mode-storage
title: 13 — Показать состояние хранилищ без чтения содержимого
status: blocked
priority: medium
model_level: medium
task_type: feature
parent_id: developer-mode-readonly
tags:
    - developer-mode
acceptance_criteria:
    - 'storage catalog/snapshot/explain покрывает sessions/memory/tasks/usage: scope, безопасные paths/sources, available/loaded и известные счётчики.'
    - Различаются repo/worktree/session/shared-corpus identity без неверного сведения разных workspaces к одному scope.
    - Чтение не сканирует историю, не запускает memory retrieval/index rebuild, task recovery или открытие/создание отсутствующего store.
    - Нет содержимого записей, названий/тел задач, memory text, session messages или raw errors; paths очищены.
    - Нет данных — unset/unavailable с причиной, а не нулевой счётчик; partial results не ломают другие категории.
verification_plan:
    - Fixtures отсутствующих/загруженных/ошибочных stores и shared memory разных worktrees.
    - Spies scan/retrieve/rebuild/open/create/write не вызываются collector; известная stale статистика отмечена.
    - Sentinel в contents/paths/errors отсутствуют в output/audit; доступные соседние категории сохраняются.
created_at: "2026-09-06T09:09:20.781113Z"
updated_at: "2026-09-06T09:09:20.781113Z"
---

## Body

**Что построить.** Безопасные сведения о состояниях уже подключённых хранилищ через harness storage. Читать epic developer-mode-readonly.

**Blocked by:** developer-mode-headless.

**Фиксированные решения.** Наблюдать текущих owners, не открывать дополнительные stores для диагностического запроса. Счётчики только известные/кэшированные с freshness; дорогую статистику не вычислять. Lexical путь или filename тоже могут содержать секрет, поэтому source использует безопасный locator и scope. Не переносить существующие memory/task инструменты в developer tool.

**Работа.** Task worktree; fixtures, временные директории только в tests; узкие checks и closeout по epic.

## Acceptance Criteria

- storage catalog/snapshot/explain покрывает sessions/memory/tasks/usage: scope, безопасные paths/sources, available/loaded и известные счётчики.
- Различаются repo/worktree/session/shared-corpus identity без неверного сведения разных workspaces к одному scope.
- Чтение не сканирует историю, не запускает memory retrieval/index rebuild, task recovery или открытие/создание отсутствующего store.
- Нет содержимого записей, названий/тел задач, memory text, session messages или raw errors; paths очищены.
- Нет данных — unset/unavailable с причиной, а не нулевой счётчик; partial results не ломают другие категории.

## Verification Plan

1. Fixtures отсутствующих/загруженных/ошибочных stores и shared memory разных worktrees.
2. Spies scan/retrieve/rebuild/open/create/write не вызываются collector; известная stale статистика отмечена.
3. Sentinel в contents/paths/errors отсутствуют в output/audit; доступные соседние категории сохраняются.

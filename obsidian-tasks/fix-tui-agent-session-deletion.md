---
id: fix-tui-agent-session-deletion
title: Восстановить удаление агентских сессий в TUI
status: done
priority: high
model_level: high
task_type: bug
tags:
    - cozyphi
    - tui
    - session
    - release-blocker
branch: bug/fix-tui-agent-session-deletion
worktree_path: .worktrees/fix-tui-agent-session-deletion
acceptance_criteria:
    - Невозможность удаления агентской сессии через TUI воспроизведена на изолированных данных и устранена.
    - Согласованная семантика закрытия/удаления безопасна для активных агентов, текущей и связанных сессий; регрессия покрыта тестом.
    - Адресные проверки зелёные, changelog обновлён, исправление интегрировано в main.
verification_plan:
    - 'Изолированный регрессионный тест реального пути UI: красный до исправления, зелёный после.'
    - Адресные тесты затронутых пакетов, race для затронутого конкурентного lifecycle.
    - Форматирование изменённых Go-пакетов и один адресный lint перед коммитом; сценарная проверка TUI.
created_at: "2026-09-06T07:36:59.801647Z"
updated_at: "2026-09-06T09:15:51.527713Z"
---

## Body

Пользователь сообщил о невозможности удаления агентских сессий в TUI; это блокирует стабилизацию и выпуск рабочей версии. Сначала воспроизвести действие на изолированных данных и определить ожидаемую семантику. Не подменять удаление закрытием вкладки и не трогать пользовательские данные. Связанная multisession-lifecycle-restore описывает более широкий lifecycle/restore и не является дублем этого дефекта.

**Started (2026-09-06).** Начата диагностика блокера стабилизации. Код и тесты — только в отдельном worktree; данные пользовательских сессий не изменять.

**Note (2026-09-06).** Пользователь согласовал закрытие вкладки с сохранением истории и результатов: ×, /close и палитра; остановка работающего агента только после подтверждения. Первая реализация прошла независимый go test -race ./cmd ./internal/tui/editor ./internal/tui/sessions ./internal/tui/controller -count=1 -timeout=120s. Ревью выявило непокрытые риски возврата focus после палитры и UI-ownership StatusHistory во время cleanup; worker на medium готовит воспроизводящие тесты и доработку. Коммитов пока нет. Реестр ведётся нативным task; MCP не требуется.

**Note (2026-09-06).** Доработчик timed_out после сохранения правок; parent проверил и продолжил сам. Новый тест через App dispatch подтверждает ввод в surviving composer после palette close. StatusHistory.BeginClose синхронно отменяет UI-owned состояние и возвращает барьер для background join. Независимый повторный -race пяти пакетов прошёл. Дополнительно Controller.BeginCloseIfIdle атомарно запирает admission с приёмом заданий; тесты обеих очередностей проходят. Запущен финальный scoped -race и единственный golangci-lint run по пяти затронутым пакетам. Ручной TTY/voice smoke остаётся непроверенным.

**Note (2026-09-06).** Проверка готовности на базе c7c5f5ae: исправление закрытия прошло -race пяти затронутых пакетов, повторные адресные тесты после usetesting-правок и git diff --check. Два независимых ревью обнаружили focus-return и UI ownership cleanup; исправлены и покрыты регрессиями. Соседняя вкладка подтверждена diff syncSelection (next, затем previous, closing пропускаются). Полный повторный lint с разрешения пользователя: /tmp/cozyphi-session-close-lint.log; новые замечания исправлены, 7 замечаний в неизменённом коде остаются. Релиз заблокирован существующими critical: defaults.go всё ещё auto-allows go test/build, mcp/config.go Load сливает project поверх user без привязки доверия к содержимому. Ручной TTY/voice smoke и полный suite не выполнялись; RC готовым не объявляем, push/публикации нет.

**Done (2026-09-06).** Исправление 5d012c4 интегрировано в main merge-коммитом fix(tui): merge retained agent tab closing. ×, /close и палитра закрывают вкладку с ID-bound подтверждением running work, async cleanup и освобождением слота только после completion, без удаления disk history/child results. Исправлены focus-return, UI retirement и idle/admission race. Scoped -race пяти затронутых пакетов PASS; финальный TestRetainedUIOwnsAcquiredHistoryUntilDisposal в task worktree PASS; связанные Plan/Gate/Hook/Restore/Recover/Cancel/Stop/Permission тесты пяти пакетов PASS. Lint повторно не запускался после разрешённого второго прогона; 7 baseline findings остаются. Релиз не готов: remove-executable-default-bash-allowlist и secure-project-mcp-config-trust актуальны; ручной TTY/voice smoke не выполнен. Версия/теги не менялись, push и публикации нет.

## Acceptance Criteria

- Невозможность удаления агентской сессии через TUI воспроизведена на изолированных данных и устранена.
- Согласованная семантика закрытия/удаления безопасна для активных агентов, текущей и связанных сессий; регрессия покрыта тестом.
- Адресные проверки зелёные, changelog обновлён, исправление интегрировано в main.

## Verification Plan

1. Изолированный регрессионный тест реального пути UI: красный до исправления, зелёный после.
2. Адресные тесты затронутых пакетов, race для затронутого конкурентного lifecycle.
3. Форматирование изменённых Go-пакетов и один адресный lint перед коммитом; сценарная проверка TUI.

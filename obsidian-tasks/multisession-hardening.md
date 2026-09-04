---
id: multisession-hardening
title: 'Мультисессия: сквозные сценарные тесты, гонки, бюджеты ресурсов, документация и CHANGELOG'
status: todo
priority: high
model_level: very_high
task_type: test
parent_id: multisession-mode
tags:
    - cozyphi
    - tui
    - test
    - multisession
branch: test/multisession-hardening
worktree_path: .worktrees/multisession-hardening
acceptance_criteria:
    - Сценарные тесты (a)–(f) существуют и зелёные; go test -race по internal/tui, internal/session, internal/tools зелёный
    - После закрытия сессии число горутин возвращается к базовому; Close всех сессий укладывается в бюджет (тест)
    - Инварианты AGENTS.md подтверждены по новым пакетам; doc/tui.md раздел Multi-session и CHANGELOG написаны
    - make fmt-check lint test в worktree зелёные
verification_plan:
    - go test -race ./internal/tui/... ./internal/session/... ./internal/tools/... в worktree
    - Живой прогон полного сценария эпика на 140 и 80 колонках, light и dark тема
    - golangci-lint run на изменённых пакетах один раз перед коммитом
created_at: "2026-09-04T07:31:55.434427Z"
updated_at: "2026-09-04T07:31:55.434427Z"
---

## Body

**Контекст:** после всех дочерних задач эпика нужен один проход по устойчивости: несколько Bus/Controller/Engine одновременно, переключения во время стримов и модалок, закрытие и рестарт.

**Что сделать:**
1. Сценарные тесты в internal/tui/editor или internal/tui/sessions с фейковым провайдером: (a) две сессии стримят одновременно, переключение туда-обратно, оба транскрипта полные; (b) permission ask в фоне, переключение, ответ уходит в источник; (c) закрытие бегущей сессии посреди tool call; (d) лимит 12 открытых; (e) restore с 3 сессиями; (f) переключение в момент `RunEndedMsg` — статус Unread/Idle корректный.
2. `go test -race` по internal/tui/..., internal/session/..., internal/tools/...; найденные гонки исправить.
3. Бюджеты: память на фоновую сессию (транскрипт), число горутин после закрытия сессии возвращается к базовому (тест с goleak или счётчиком), Close всех сессий укладывается в общий бюджет.
4. Аудит инвариантов AGENTS.md по всем новым пакетам: без обратных указателей на Editor, конструкторы с параметрами, отсутствие Deps-мешков, дедупликация логики статусов.
5. doc/tui.md: раздел «Multi-session» (архитектура ярусов, панель, клавиши, статусы, восстановление), таблица агрегации сообщений; README — краткое упоминание; CHANGELOG Unreleased сводная запись по эпику.

**Blocked by:** multisession-title-tool, multisession-switch-cues, multisession-background-attention, multisession-projects, multisession-lifecycle-restore

## Acceptance Criteria

- Сценарные тесты (a)–(f) существуют и зелёные; go test -race по internal/tui, internal/session, internal/tools зелёный
- После закрытия сессии число горутин возвращается к базовому; Close всех сессий укладывается в бюджет (тест)
- Инварианты AGENTS.md подтверждены по новым пакетам; doc/tui.md раздел Multi-session и CHANGELOG написаны
- make fmt-check lint test в worktree зелёные

## Verification Plan

1. go test -race ./internal/tui/... ./internal/session/... ./internal/tools/... в worktree
2. Живой прогон полного сценария эпика на 140 и 80 колонках, light и dark тема
3. golangci-lint run на изменённых пакетах один раз перед коммитом

---
id: refactor-composer-routing-seam
title: 'Композер: routing-seam вместо Wire из 14 колбэков и commandBridge из 15'
status: done
priority: high
task_type: refactor
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - Wire/bridge заменены interface с двумя adapter-ами
    - композер тестируется без full-app
    - HookCommands собирается конструктором полностью
verification_plan:
    - go test ./internal/tui/composer/... (новые)
created_at: "2026-08-23T15:17:22.120559Z"
updated_at: "2026-08-23T19:10:24.870982Z"
---

## Body

ComposerPane.Wire(...) 14 параметров (pane.go:72-87); newCommandBridge 15 (editor.go:537-554); CommandContext — 13-функциональный deps-bag с одним продюсером (commands/registry.go:16-37); HookCommands — публичные поля, мутируемые после конструктора (editor.go:123-131 строит, :168 ставит Submitter, :187 ставит CommandCtx, :221 первое использование) — двухфазная половинная инициализация против правил AGENTS. Кандидат: один маленький interface (submit/cancel/overlay-keys/focus), Editor — первый adapter, fake — второй в тестах. Заодно закрывает ноль-тесты композера (645 строк Esc-ladder/picker arbitration без единого теста).

## Acceptance Criteria

- Wire/bridge заменены interface с двумя adapter-ами
- композер тестируется без full-app
- HookCommands собирается конструктором полностью

## Verification Plan

1. go test ./internal/tui/composer/... (новые)

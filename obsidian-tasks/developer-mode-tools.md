---
id: developer-mode-tools
title: 06 — Объяснить регистрацию и доступность инструментов
status: blocked
priority: medium
model_level: medium
task_type: feature
parent_id: developer-mode-readonly
tags:
    - developer-mode
acceptance_criteria:
    - harness tools перечисляет реально зарегистрированные инструменты и безопасную причину отсутствия/ограничения известных инструментов.
    - Registered, допустимость по роли/плану и необходимость argument-dependent permission check представлены раздельно.
    - Смена шага, model/tool rebind и отсутствие MCP/LSP/job manager отражаются по текущему владельцу без устаревшего cached tool list.
    - Read не вызывает инструмент, preflight чужого вызова, Ask, plan transition или выдачу capabilities.
    - Не экспортируются MCP server schemas, raw tool payloads и sensitive arguments; только нативная метаинформация.
verification_plan:
    - Engine fixture с отсутствующими и присутствующими managers; сопоставить реальный tool list и диагностику.
    - Переход шага и rebind между снимками обновляет причины; нет пробного вызова инструментов.
    - Argument-dependent gate не сводится к безусловному allowed; sentinel payload/schema не просачиваются.
created_at: "2026-09-06T09:07:38.767392Z"
updated_at: "2026-09-06T09:07:38.767392Z"
---

## Body

**Что построить.** По harness можно понять, почему инструмент виден/не виден и что ограничивает его использование сейчас. Читать epic developer-mode-readonly.

**Blocked by:** developer-mode-permissions.

**Фиксированные решения.** Не утверждать allowed для всех аргументов только на основании имени. Использовать понятные состояния registered, unavailable, restricted и requires_argument_check с причинами из владельцев; никакого второго permission evaluator. State плана можно наблюдать через существующий snapshot, не ждать отдельного collector developer-mode-plan. Базовый перечень известных возможностей не импортирует MCP schemas в контекст.

**Работа.** Изменять только introspection и подключение к harness в task worktree; профильные тесты и closeout по epic.

## Acceptance Criteria

- harness tools перечисляет реально зарегистрированные инструменты и безопасную причину отсутствия/ограничения известных инструментов.
- Registered, допустимость по роли/плану и необходимость argument-dependent permission check представлены раздельно.
- Смена шага, model/tool rebind и отсутствие MCP/LSP/job manager отражаются по текущему владельцу без устаревшего cached tool list.
- Read не вызывает инструмент, preflight чужого вызова, Ask, plan transition или выдачу capabilities.
- Не экспортируются MCP server schemas, raw tool payloads и sensitive arguments; только нативная метаинформация.

## Verification Plan

1. Engine fixture с отсутствующими и присутствующими managers; сопоставить реальный tool list и диагностику.
2. Переход шага и rebind между снимками обновляет причины; нет пробного вызова инструментов.
3. Argument-dependent gate не сводится к безусловному allowed; sentinel payload/schema не просачиваются.

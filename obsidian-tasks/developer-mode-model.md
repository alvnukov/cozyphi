---
id: developer-mode-model
title: 03 — Показать configured/loaded/effective модель и источники
status: done
priority: medium
model_level: medium
task_type: feature
parent_id: developer-mode-readonly
tags:
    - developer-mode
acceptance_criteria:
    - harness snapshot model и explain поддержанных model-полей показывают выбранную и фактически действующую модель, effort, context window и capabilities.
    - Явно представлены configured/loaded/effective и применимые default/config/ENV/session/plan sources с порядком overrides.
    - Изменение config на диске не меняет loaded/effective и не вызывает reload; сохранённый default не выдаётся за фактическую модель.
    - Для неизвестного происхождения explicit unavailable, для неподдержанного effort не фиктивное значение; apply semantics обоснованы поведением владельца.
    - DTO не содержит API keys, raw model config или неочищенные endpoint/error strings; результаты detached.
verification_plan:
    - Table tests default/config/ENV/session/plan overrides и неподдержанного effort.
    - 'Сценарий: загрузить A, изменить disk на B, затем изменить session на C; snapshot объясняет каждый слой без reload.'
    - Interface tests sentinel credentials, detached maps/slices и parity TUI/headless при одинаковом состоянии.
created_at: "2026-09-06T09:06:41.967581Z"
updated_at: "2026-09-06T11:06:13.470449Z"
---

## Body

**Что построить.** Добавить завершённую model-категорию для настроек выбора модели и фактического состояния engine через существующий harness. Читать epic developer-mode-readonly.

**Blocked by:** developer-mode-headless.

**Фиксированные решения.** Фактическое состояние берётся у владельца engine, конфигурация — у загрузчика с явной provenance. Не вычислять effective повторным применением конфигурации. Если provenance теряется при загрузке, сохранить минимальные безопасные метаданные именно в пути загрузки; не вводить второй загрузчик или mutable копию настроек. Provider credentials и каталог импортов не входят в эту задачу: developer-mode-providers расширит ту же категорию.

**Scope.** Модель, effort, окно, поддерживаемые модельные возможности и их источники. Для точного полного перечня сверить действующие поля загрузчика; исключения отразить в catalog. Чтение новых файлов настроек только без публикации/reload и с теми же validation/trust rules.

**Работа.** Task worktree, узкие тесты и closeout по epic.

## Acceptance Criteria

- harness snapshot model и explain поддержанных model-полей показывают выбранную и фактически действующую модель, effort, context window и capabilities.
- Явно представлены configured/loaded/effective и применимые default/config/ENV/session/plan sources с порядком overrides.
- Изменение config на диске не меняет loaded/effective и не вызывает reload; сохранённый default не выдаётся за фактическую модель.
- Для неизвестного происхождения explicit unavailable, для неподдержанного effort не фиктивное значение; apply semantics обоснованы поведением владельца.
- DTO не содержит API keys, raw model config или неочищенные endpoint/error strings; результаты detached.

## Verification Plan

1. Table tests default/config/ENV/session/plan overrides и неподдержанного effort.
2. Сценарий: загрузить A, изменить disk на B, затем изменить session на C; snapshot объясняет каждый слой без reload.
3. Interface tests sentinel credentials, detached maps/slices и parity TUI/headless при одинаковом состоянии.

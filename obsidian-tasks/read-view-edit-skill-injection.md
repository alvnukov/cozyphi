---
id: read-view-edit-skill-injection
title: Разделить view/edit чтение и автоматически инжектить skills
status: done
priority: high
task_type: feature
tags:
    - agent
    - tools
    - context
    - skills
    - tokens
acceptance_criteria:
    - Обычный read по умолчанию возвращает строки без @file TAG и LINE#HASH, сохраняя номера строк.
    - Явный edit-режим read выдаёт hashline-якоря, пригодные для fail-closed edit.
    - Edit отклоняет якоря, если в текущей сессии не было соответствующего editable read/grep.
    - Выбранные plan-step skills автоматически инжектятся полным plain-text содержимым до первого рабочего вызова шага.
    - Обновлены tool guidance, документация и CHANGELOG; make fmt-check lint test проходят.
verification_plan:
    - Целевые unit/integration tests для read schema/output, edit authorization и skill injection.
    - make fmt-check lint test.
    - Параллельный standards/spec review перед интеграцией.
created_at: "2026-08-31T09:45:23.383323Z"
updated_at: "2026-08-31T09:46:05.925491Z"
---

## Body

**Контекст:** Hashline-разметка нужна только для подготовки edit, но сейчас read добавляет её ко всем файлам, включая SKILL.md. Plan-step skill injection пока передаёт модели инструкцию вручную прочитать SKILL.md.

**Решение:** Разделить view-read и editable read без переходного периода. View становится default. Skill body загружает harness при старте шага как контекстный ресурс без hashline. Memory и project instructions остаются на прямом raw-loading пути.

**Ограничения:** Не вводить path-based исключения для каталога skills. Сохранить fail-closed hashline edit и не затронуть чужие изменения.

## Acceptance Criteria

- Обычный read по умолчанию возвращает строки без @file TAG и LINE#HASH, сохраняя номера строк.
- Явный edit-режим read выдаёт hashline-якоря, пригодные для fail-closed edit.
- Edit отклоняет якоря, если в текущей сессии не было соответствующего editable read/grep.
- Выбранные plan-step skills автоматически инжектятся полным plain-text содержимым до первого рабочего вызова шага.
- Обновлены tool guidance, документация и CHANGELOG; make fmt-check lint test проходят.

## Verification Plan

1. Целевые unit/integration tests для read schema/output, edit authorization и skill injection.
2. make fmt-check lint test.
3. Параллельный standards/spec review перед интеграцией.

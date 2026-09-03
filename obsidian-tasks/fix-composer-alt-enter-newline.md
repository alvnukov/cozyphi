---
id: fix-composer-alt-enter-newline
title: 'Alt+Enter перевод строки (корень: vendored парсер)'
status: done
priority: medium
task_type: bug
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - Alt+Enter даёт перевод строки
    - дивергенция записана в PATCH_NOTES.md
verification_plan:
    - ручная проверка в TUI
created_at: "2026-08-23T14:45:57.937125Z"
updated_at: "2026-09-02T19:01:40.124134Z"
---

## Body

Alt+Enter в композере отправляет сообщение вместо перевода строки; C-u не очищает композер. Корневая причина (ревью 2026-08-23): вендорный парсер xui/input/parser.go:193-210 — legacy ESC+byte путь синтезирует ModAlt только для b[1]>=0x20; ESC CR декодируется как KeyEscape+KeyEnter, никогда Enter+ModAlt. chat_input.go:186 ModAlt обрабатывает корректно. Фикс в парсере + зафиксировать дивергенцию в xui/PATCH_NOTES.md.

## Acceptance Criteria

- Alt+Enter даёт перевод строки
- дивергенция записана в PATCH_NOTES.md

## Verification Plan

1. ручная проверка в TUI

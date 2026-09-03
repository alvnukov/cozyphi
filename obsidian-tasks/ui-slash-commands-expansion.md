---
id: ui-slash-commands-expansion
title: Расширить набор слэш-команд
status: done
priority: medium
task_type: feature
parent_id: cozyphi-convenience-program
tags:
    - ui
    - commands
acceptance_criteria:
    - команды разбираются с аргументами
    - autocomplete-меню по мере ввода
    - не конфликтует с обычным текстом
verification_plan:
    - юнит-тесты парсера команд
    - ручная проверка в tmux
created_at: "2026-08-23T14:45:57.933603Z"
updated_at: "2026-08-23T22:57:23.405669Z"
---

## Body

По итогам сравнения с opencode: /new (чистая сессия), /compact (сжатие контекста), /export, /theme. Команды должны срабатывать и не только на пустой строке.

## Acceptance Criteria

- команды разбираются с аргументами
- autocomplete-меню по мере ввода
- не конфликтует с обычным текстом

## Verification Plan

1. юнит-тесты парсера команд
2. ручная проверка в tmux

---
id: fix-long-prompt-input
title: Устранить ограничение длинного промпта в редакторе ввода
status: done
priority: high
model_level: medium
task_type: bug
tags:
    - tui
branch: bug/fix-long-prompt-input
worktree_path: .worktrees/fix-long-prompt-input
acceptance_criteria:
    - Вставка больше 1 МиБ доходит до ChatInput и OnSubmit без потери текста и преждевременной отправки.
    - Регрессионные тесты покрывают границу 1 МиБ и разрыв завершающего маркера.
verification_plan:
    - Регрессии Parser.Feed на 1 МиБ ±1 байт и 2 МиБ; ChatInput → OnSubmit.
    - Тесты xui/input, chat, composer, submit; scoped lint input и chat.
created_at: "2026-09-05T21:54:33.644414Z"
updated_at: "2026-09-05T22:08:51.410847Z"
---

## Body

Подтверждён дефект Parser.Feed: вставка принудительно завершалась после 1 МиБ, остаток становился нажатиями клавиш. Удалено преждевременное завершение, добавлены регрессии парсера и ChatInput → OnSubmit. Размер исходного пользовательского промпта неизвестен; исправление подтверждено для этого воспроизведения. Компромисс: незавершённая вставка может накапливать память без порога парсера.

**Done (2026-09-06).** Исправление a35af4e влито в main merge-коммитом 8cc90fd; конфликт только CHANGELOG, сохранены обе записи. Удалён force-end вставки >1 МиБ; добавлены тесты границы Parser.Feed и целостной отправки ChatInput. Тесты input/chat/composer/submit прошли, форматирование выполнено, два review без замечаний. Scoped lint выявил 5 старых замечаний в неизменённых строках xui/input: заведена xui-input-lint-debt. Исходный пользовательский размер не установлен, подтверждена именно регрессия >1 МиБ.

## Acceptance Criteria

- Вставка больше 1 МиБ доходит до ChatInput и OnSubmit без потери текста и преждевременной отправки.
- Регрессионные тесты покрывают границу 1 МиБ и разрыв завершающего маркера.

## Verification Plan

1. Регрессии Parser.Feed на 1 МиБ ±1 байт и 2 МиБ; ChatInput → OnSubmit.
2. Тесты xui/input, chat, composer, submit; scoped lint input и chat.

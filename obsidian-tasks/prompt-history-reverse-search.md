---
id: prompt-history-reverse-search
title: 'Ctrl+R: обратный инкрементальный поиск по истории промптов (reverse-i-search)'
status: in_progress
priority: medium
task_type: feature
parent_id: cozyphi-convenience-program
tags:
    - tui
    - composer
branch: feature/prompt-history-reverse-search
worktree_path: .worktrees/prompt-history-reverse-search
acceptance_criteria:
    - 'Ctrl+R в композере включает reverse-i-search: инкрементальный фильтр истории без учёта регистра, повторный Ctrl+R листает совпадения к более старым, Ctrl+S — к более новым'
    - Enter отправляет найденный промпт немедленно (единый путь OnSubmit, запись в историю); Esc оставляет совпадение в буфере; Ctrl+G отменяет поиск и восстанавливает исходный ввод (пока поиск активен — побеждает голосовой Ctrl+G)
    - 'Стрелки и ввод текста ведут себя как в bash: редактирование запроса не трогает буфер, стрелки выходят из поиска с совпадением в буфере'
    - Видна строка поиска с запросом и превью совпадения; существующие биндинги композера не сломаны
    - make fmt-check lint test зелёные; строка в CHANGELOG.md; обновлён doc/tui.md
verification_plan:
    - go test ./internal/components/chat/ ./internal/history/ ./internal/tui/composer/
    - 'Ручная проверка: Ctrl+R → печать запроса → повторный Ctrl+R листает; Enter отправляет; Esc оставляет в буфере; Ctrl+G восстанавливает; Ctrl+G вне поиска включает голос'
    - make fmt-check lint test
    - Проверить, что ~/.cozyphi/prompt-history.jsonl читается после отправки найденного
created_at: "2026-09-03T17:26:53.840372Z"
updated_at: "2026-09-03T17:26:59.186271Z"
---

## Body

Семантика один в один как в bash reverse-i-search, адаптированная к чат-композеру:
- **Ctrl+R** (аккорд через keys-таблицу, keys.CmdHistorySearch) включает режим; повторный Ctrl+R — совпадение старше; **Ctrl+S** — моложе.
- **Enter** — сразу отправить найденный промпт (через Chat.OnSubmit — единый путь сабмита, попадает в prompt-history).
- **Esc** — принять совпадение в буфер (редактирование вторым Enter).
- **Ctrl+G** — отмена с восстановлением исходного буфера. Конфликт с голосом (keys.CmdVoice): пока поиск активен, Ctrl+G — отмена поиска.
- Печать/Backspace редактируют **запрос**, не буфер; стрелки выходят из поиска, оставив совпадение в буфере.

**Архитектура** (по разведке 2026-09-03): состояние режима — в `internal/components/chat` (новый файл search.go: query, matches, idx, saved value/cursor); стор `internal/history.Store` расширяется методом `Search(query) []string` (newest-first, case-insensitive substring) — расширить интерфейс `chat.Recaller` или добавить второй маленький шов; вход в поиск делает `Store.Reset()`. Аккорды Ctrl+R/Ctrl+S обрабатываются на уровне `ComposerPane.Handle` через keys-таблицу (как CmdPalette). Рендер: meta row показывает поиск, тело — превью совпадения с подсветкой. Стор уже персистентный: ~/.cozyphi/prompt-history.jsonl, 50 записей — не менять.

**Скиллы шагов плана обязательны к загрузке до начала шага.**

## Acceptance Criteria

- Ctrl+R в композере включает reverse-i-search: инкрементальный фильтр истории без учёта регистра, повторный Ctrl+R листает совпадения к более старым, Ctrl+S — к более новым
- Enter отправляет найденный промпт немедленно (единый путь OnSubmit, запись в историю); Esc оставляет совпадение в буфере; Ctrl+G отменяет поиск и восстанавливает исходный ввод (пока поиск активен — побеждает голосовой Ctrl+G)
- Стрелки и ввод текста ведут себя как в bash: редактирование запроса не трогает буфер, стрелки выходят из поиска с совпадением в буфере
- Видна строка поиска с запросом и превью совпадения; существующие биндинги композера не сломаны
- make fmt-check lint test зелёные; строка в CHANGELOG.md; обновлён doc/tui.md

## Verification Plan

1. go test ./internal/components/chat/ ./internal/history/ ./internal/tui/composer/
2. Ручная проверка: Ctrl+R → печать запроса → повторный Ctrl+R листает; Enter отправляет; Esc оставляет в буфере; Ctrl+G восстанавливает; Ctrl+G вне поиска включает голос
3. make fmt-check lint test
4. Проверить, что ~/.cozyphi/prompt-history.jsonl читается после отправки найденного

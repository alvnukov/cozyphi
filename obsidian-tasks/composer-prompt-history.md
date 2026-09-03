---
id: composer-prompt-history
title: Composer prompt history + placeholder panel background
status: done
priority: high
tags:
    - tui
    - composer
    - phase-1
verification_plan:
    - go test ./internal/history/...
    - go test ./internal/components/chat/...
    - go test ./internal/tui/composer/...
    - make fmt-check lint test из worktree
    - 'ручная проверка: Up в пустом композере подставляет последний промпт'
created_at: "2026-08-24T15:21:35.703665Z"
updated_at: "2026-08-24T15:44:52.178671Z"
---

## Body

Две жалобы одним заходом (opencode-паритет, ground truth: ~/src/opencode packages/tui/src/prompt/history.tsx).

1. История ввода: Up при курсоре в начале / Down при конце композера гуляет по отправленным промптам (index 0 = черновик; дивергенция текста блокирует прогулку; Down возвращает черновик). Хранение: ~/.phi/prompt-history.jsonl, JSON-строки, лимит 50, дедуп подряд, append-only (rewrite при триме/самолечении). Новый leaf-пакет internal/history (Store: Open/Append/Prev/Next/Reset, nil-толерантный). ChatInput.History — интерфейс-шов Recaller; ComposerPane: NewComposerPane(..., *history.Store), OnSubmit → Append, ClearInput → Reset; cmd/main.go открывает стор и пробрасывает в NewEditor.

2. Фон плейсхолдера: Surface.Print затирает стиль ячейки целиком — плейсхолдер (th.Muted), набранный текст (Foreground), мета-ряд и skills теряют Bg панели BackgroundElement (#1e1e1e) и пробивают дыры дефолтного фона. Фикс в ChatInput.Draw: всем стилям внутри рамки проставляется Bg=panelBg (hints-ряд вне панели — без изменений). Обновить пины в chat_test.go (placeholder style, meta lead style).

## Verification Plan

1. go test ./internal/history/...
2. go test ./internal/components/chat/...
3. go test ./internal/tui/composer/...
4. make fmt-check lint test из worktree
5. ручная проверка: Up в пустом композере подставляет последний промпт

---
id: safe-paste-limit
title: Безопасно ограничить текстовую вставку одним МиБ
status: done
priority: medium
model_level: medium
task_type: bug
tags:
    - tui
branch: bug/safe-paste-limit
worktree_path: .worktrees/safe-paste-limit
acceptance_criteria:
    - Текстовая вставка до 1 МиБ принимается; больше — отказ целиком с уведомлением без изменения черновика.
    - Хвост отклонённой вставки не интерпретируется как клавиши; память накопления ограничена.
    - Изображения не ограничены текстовым порогом.
verification_plan:
    - Пограничные и chunked тесты парсера.
    - Тест отказа с сохранением черновика и проверки изображений.
    - Scoped format/test/lint.
created_at: "2026-09-05T23:29:43.308415Z"
updated_at: "2026-09-05T23:39:31.210155Z"
---

## Body

Вернуть безопасный предел 1 МиБ после fix-long-prompt-input. Не force-end: отбрасывать превышающую лимит вставку до терминального маркера, уведомлять пользователя. Путь изображений проверить отдельно.

**Note (2026-09-06).** Implemented raw-byte limit and drain-until-marker rejection event, global warning, exact-boundary/fragmented-marker/recovery/draft/image regressions. Initial scoped input/chat/composer/editor tests pass. Two-axis review: no must-fix findings. Final scoped format/test/lint started as watch w4. Harness LSP dependency diagnostics mismatch recorded separately as lsp-worktree-dependency-diagnostics.

**Done (2026-09-06).** Delivered code d8b0dbf and merged --no-ff into main. Accepts <=1048576 raw bytes; rejects larger terminal text pastes whole, drains actual closing marker, warns without changing draft. Images >1MiB covered and unaffected. Scoped tests input/chat/composer/editor/submit and formatting pass; root scoped lint passes, xui retains exactly five known xui-input-lint-debt findings. Standards/spec reviews: no must-fix findings. LSP false fresh dependency errors registered separately. No push.

## Acceptance Criteria

- Текстовая вставка до 1 МиБ принимается; больше — отказ целиком с уведомлением без изменения черновика.
- Хвост отклонённой вставки не интерпретируется как клавиши; память накопления ограничена.
- Изображения не ограничены текстовым порогом.

## Verification Plan

1. Пограничные и chunked тесты парсера.
2. Тест отказа с сохранением черновика и проверки изображений.
3. Scoped format/test/lint.

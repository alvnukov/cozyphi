---
id: fix-plan-editor-model-ranking
title: Исправить ранжирование моделей в редакторе плана
status: done
priority: medium
model_level: medium
task_type: bug
tags:
    - tui
    - plan
branch: bug/fix-plan-editor-model-ranking
worktree_path: .worktrees/fix-plan-editor-model-ranking
acceptance_criteria:
    - В редакторе плана список моделей использует тот же порядок ранжирования, что и остальные пикеры моделей.
    - Существующие сценарии выбора модели не регрессируют.
    - Добавлен или обновлён тест, фиксирующий единый порядок.
verification_plan:
    - Запустить целевые тесты компонента/редактора плана.
    - Запустить make fmt-check lint test.
created_at: "2026-09-05T11:00:33.123816Z"
updated_at: "2026-09-05T11:38:16.440231Z"
---

## Body

Пикер моделей в редакторе плана показывает модели в ином порядке, чем остальные пикеры. Нужно найти общий механизм ранжирования и применить его без дублирования логики.

**Started (2026-09-05).** Начинаю диагностику и исправление в отдельном task-worktree.

**Done (2026-09-05).** Implemented in f3fb961, merged into main in c146d8a. Plan now uses shared history-bound model/effort picker; effort relevance is per model including default and credits only successful choices. Regression tests cover draft/cancel/type defaults and ranking isolation. Scoped format, go build and go test passed for commands/editor/planedit/usage. One scoped lint found only unparam on the required error-returning test callback; documented narrow suppression added and commands tests passed again (lint not rerun). Independent Spec review clean; Standards history-error finding deliberately retained as existing best-effort usage policy. CHANGELOG merge preserved both concurrent additions. Unrelated go.sum excluded.

## Acceptance Criteria

- В редакторе плана список моделей использует тот же порядок ранжирования, что и остальные пикеры моделей.
- Существующие сценарии выбора модели не регрессируют.
- Добавлен или обновлён тест, фиксирующий единый порядок.

## Verification Plan

1. Запустить целевые тесты компонента/редактора плана.
2. Запустить make fmt-check lint test.

---
id: diff-block-claude-style
title: Дифф в транскрипте как у Claude Code — номера, маркеры, фон строки, подсветка, выделение только текста
status: in_progress
priority: high
model_level: high
task_type: feature
tags:
    - tui
    - transcript
    - ux
branch: feature/diff-block-claude-style
worktree_path: .worktrees/diff-block-claude-style
acceptance_criteria:
    - Тело диффа рисуется построчно: колонка номера строки (новый файл для контекста и +, старый для −), колонка маркера `+`/`−`/пробел, затем код. Фон на всю ширину строки: зелёный тон для +, красный для −, панельный для контекста; текст кода подсвечен chroma по расширению файла из заголовка диффа, маркеры и номера — приглушённые. Заголовки `---`/`+++` не показываются, `@@` — тонкий разделитель между ханками.
    - Длинные строки обрезаются по ширине, не переносятся (у диффа перенос ломает чтение); обрезка отмечается `…`.
    - Выделение мышью в транскрипте подсвечивает и копирует только текстовые ячейки: колонки номеров, маркеров и гаттер-бар блока помечены как chrome и не попадают ни в подсветку, ни в буфер. CopyText диффа отдаёт код без маркеров? — нет: CopyText блока по-прежнему отдаёт unified diff (это осознанная копия патча), а drag-выделение отдаёт чистый текст строк.
    - Механизм chrome живёт в components (Surface-маска + помощник для блоков), gutterBar помечает свою колонку — все блоки перестают копировать бар.
    - CHANGELOG Unreleased; хинты не меняются.
verification_plan:
    - Тесты diff_block: раскладка колонок, номера по ханкам, фон строки, обрезка, подсветка по расширению, отсутствие ---/+++.
    - Тесты selection: ExtractSurfaceText и ApplySelectionHighlight пропускают chrome; вложенные Surface сохраняют маску.
    - Гейты только по изменённым пакетам; один прогон golangci-lint.
created_at: "2026-09-07T09:55:00Z"
updated_at: "2026-09-07T09:55:00Z"
---

## Body

**Откуда.** Скриншот диффа Claude Code от пользователя 2026-09-07: `361 +}` … `392   h := newHarness(Row{` — номер, маркер, код с подсветкой синтаксиса, зелёный фон на всю строку у добавленных, контекст на обычном фоне, длинная строка 373 обрезана по краю. Выделение мышью (строки 375–384) начинается после колонки маркера и в буфер идёт только код. У нас `diffBodyLines` красит сырые строки unified diff целиком зелёным/красным текстом с переносом, а выделение и копирование берут все ячейки, включая гаттер и `+`/`-`.

**Точки входа.** `internal/components/block/diff_block.go` (Draw, diffBodyLines, CopyText), `internal/components/block/inset.go` (gutterBar), `internal/components/selection.go` (ExtractSurfaceText, applySelHighlight, flattenSurface), `internal/components/surface.go` (Surface), `internal/components/text/markdown.go` (как уже используется chroma), `internal/tui/transcript/pane.go:693` (drag copy).

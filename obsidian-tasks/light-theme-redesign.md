---
id: light-theme-redesign
title: 'Светлая тема: цельный редизайн палитры по мотивам VS light'
status: in_progress
priority: high
task_type: feature
tags:
    - ui
    - theme
branch: feature/light-theme-redesign
worktree_path: .worktrees/light-theme-redesign
acceptance_criteria:
    - 'Светлая тема — согласованное целое: синтаксис (diff, код-блоки, tool-вывод), UI-панели и текст читаемы, без слепых и серых-на-белом сочетаний'
    - Референс подсветки — Visual Studio light (C/C++)
    - Тёмная тема не деградировала
    - Пользователь подтвердил внешний вид
    - Коммит смержен в main, CHANGELOG дополнен, воркдрей и ветка удалены
verification_plan:
    - go build + go test по изменённым пакетам
    - gofmt -l по изменённым каталогам, один golangci-lint run перед коммитом
    - Переключение светлой/тёмной темы — обе живы
    - Визуальная проверка транскрипта с diff, код-блоками, tool-выводом и панелями — подтверждение пользователя
    - 'CHANGELOG: строка под [Unreleased]'
created_at: "2026-09-05T08:14:57.396933Z"
updated_at: "2026-09-05T08:39:30.471335Z"
---

## Body

Светлая тема cozyphi сейчас «убожество» (цитата пользователя): несогласованная палитра,
слабый контраст, элементы, рассчитанные на тёмный фон.

**Объём:** всё сразу — подсветка кода (diff, код-блоки, вывод тулов), палитра UI
(панели, рамки, статус-бар, акценты), читаемость текста транскрипта.

**Референс:** Visual Studio light — классическая C/C++ подсветка VS.

**План работ:** обследование системы тем → семантическая таблица цветов
(согласовать с пользователем) → правки в воркдрее → визуальная проверка →
гейты по изменённым пакетам, коммит, merge в main, уборка воркдрея.

**Note (2026-09-05).** Survey (step survey-theme): тема — один модуль internal/components/theme.go: Theme (~40 слотов: chrome, Selection/Picker, Secondary/BackgroundPanel/BackgroundElement, MarkdownRoles 11, SyntaxRoles 9), 6 builtin-тем, ThemeByName, выбор через /theme (tui/commands/builtins.go, editor.go:1517). Потребители: components/block/* (diff, tool, bash, agent, status), components/text/markdown*.go (chroma → SyntaxRoles через chromaStyle, markdown.go:359), status, toast, splash, chat, input. В internal/tui хардкода RGB нет (единственный — chat_input.go:989 IndexedColor(240)).

Дефекты opencode-light: (1) BackgroundPanel #fafafa/BackgroundElement #f5f5f5 неотличимы от белого терминала — панели и подложка diff невидимы; (2) Muted #8a8a8a ≈3:1 на белом; (3) синтаксис opencode-производный, не VS: keyword янтарный #d68c27, type оливковый #b0851f; (4) diff — только Fg green/red на невидимой подложке, без VS-тонов; (5) Warning=keyword=heading=strong — всё янтарь; (6) BlockHighlight #e8e4da почти невидим. Diff красится в diff_block.go diffBodyLines: +→Success, −→Destructive, @@→Secondary, контекст dim; hunks на BackgroundPanel (FillRowsBg).

**Note (2026-09-05).** Design (step design-palette), подтверждено пользователем: (1) diff — VS-тинты: новые роли Theme.DiffAdd/DiffRemove/DiffMeta; light: DiffAdd Fg #107C10 на Bg #CCFFCC (≈4.8:1), DiffRemove Fg #A4262C на Bg #FFCCCC (≈5.1:1), DiffMeta Fg #0451A5; для остальных тем наследуются из Success/Destructive/Secondary без Bg (тёмная не меняется). (2) Тема переименована: display "Light (VS)", func VSLightTheme, aliases "opencode-light"/"vs-light"/"light" для совместимости. Палитра: синтаксис VS C/C++ — keyword #0000FF, comment #008000, string #A31515, number #09885A, type #2B91AF, function #795E26, variable/operator/punct #1F1F1F; хром — Foreground #1F1F1F, Muted #666666, Border #BFBFBF, BackgroundPanel #ECECEC, BackgroundElement #F5F5F5, Accent/SelectionBg #0078D4, ToolName/Keybind/Command #0451A5/#0078D4, Violet #68217A, Success #107C10, Warning #8A5A00, Destructive #A4262C, BlockHighlight #FFF3C4; markdown — heading #0078D4 bold, strong/emph #1F1F1F, inline-code #A31515, link #0078D4, quote #666666, code #1F1F1F. Пикер — синий бар #0078D4 + белый. Точный контраст-чек скриптом в шаге правок.

**Note (2026-09-05).** implement-palette done: в воркдрее .worktrees/light-theme-redesign (branch feature/light-theme-redesign) — Theme получил DiffAdd/DiffRemove/DiffMeta (VS Light: заливка #CCFFCC/#FFCCCC; остальные темы наследуют цвета текста без заливки), opencode-light → vs-light (aliases: opencode-light, light, vs light), diff_block.go красит строки диффа построчно, обновлены theme_test.go, commands_expansion_test.go, doc/tui.md. go build + go test по всем затронутым пакетам зелёные, gofmt чист, контраст пар ≥4.5:1 (кроме type 3.64:1 и number 4.49:1 — верные значения VS). Превью-бинарь: /tmp/phi-vs-light (сборка ./cmd из воркдрея). Осталось: visual-verify (пользователь смотрит /theme vs-light), finish-and-merge (CHANGELOG, scoped lint, коммит, merge --no-ff, уборка).

## Acceptance Criteria

- Светлая тема — согласованное целое: синтаксис (diff, код-блоки, tool-вывод), UI-панели и текст читаемы, без слепых и серых-на-белом сочетаний
- Референс подсветки — Visual Studio light (C/C++)
- Тёмная тема не деградировала
- Пользователь подтвердил внешний вид
- Коммит смержен в main, CHANGELOG дополнен, воркдрей и ветка удалены

## Verification Plan

1. go build + go test по изменённым пакетам
2. gofmt -l по изменённым каталогам, один golangci-lint run перед коммитом
3. Переключение светлой/тёмной темы — обе живы
4. Визуальная проверка транскрипта с diff, код-блоками, tool-выводом и панелями — подтверждение пользователя
5. CHANGELOG: строка под [Unreleased]

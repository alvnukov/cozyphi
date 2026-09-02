---
id: feed-semantic-tool-rows
title: Semantic per-tool feed rows instead of one generic tool block
status: done
tags:
    - ui
    - ux-standard
    - feed
created_at: "2026-09-01T17:22:34.000000Z"
updated_at: "2026-09-01T17:45:31.000000Z"
---

## Body

**Проблема.** Лента синтаксическая, а не семантическая: все тулы, кроме bash и агентов, рендерятся одним генерик-`ToolBlock` — «глиф + имя + detail + сырой output» (`internal/tui/transcript/mapper.go` `patchTool`/`toolWidget`, `internal/components/block/tool_block.go`). Правка файла выглядит так же, как grep; diff-рендерера в `internal/components` нет вообще; read на 500 строк и правка, сломавшая файл, визуально равноценны. Пользователь не видит главного для контроля — что именно изменилось.

**Что сделать.** Реестр per-tool рендереров вместо генерик-блока. (1) edit/write — diff-карточка: заголовок-путь, окрашенные +/− строки, статы «+12 −3»; карточка inline у текущего/последнего хода, у старых сворачивается до строки со статами. (2) read/grep/ls/find/lsp — семантические однострочники без тела по умолчанию: «read pane.go (641 lines)», «grep: 14 matches in 6 files»; тело по Enter. (3) mcp-тулы — именование `server · tool`. (4) Ошибки никогда не сворачиваются. Неизвестный тул — прежний генерик-путь.

**Критерий.** Edit в ленте показывает diff-карточку со статами; read/grep — одну смысловую строку, разворачиваемую по Enter; mcp-строка называет сервер; ошибка тула видна без разворачивания; неизвестный тул рендерится как раньше.

**Результат.** Сделано (commit cf4fb4e, merge в main 866ee94). (1) Diff-карточка `block.DiffBlock`: заголовок «глиф edit/write путь +N −M ▶» — статы всегда в заголовке, даже свёрнутой; тело — окрашенные ханки (+ зелёный, − красный, @@ secondary, контекст dim), file-header ---/+++ отброшен как дубль пути; `DiffStats` считает по префиксам. Роутинг в mapper (`isDiffTool` edit/write) с фолбэком: незнакомый тул — прежний генерик-`ToolBlock`. (2) Карточки текущего хода открываются сами и складываются до стат-строки при завершении хода: `currentTurnTools` собирает tool_use-id после последнего отправленного user-сообщения (queued не граница), DiffBlock исключён из snapshot-цикла expand-состояний — явный toggle пользователя (через OnToggle) переживает оба правила; закреплено тестами mapper_diff_test.go (4 шт: роутинг+auto-open+fold, queued не складывает, явный toggle живёт, write→карточка/grep→генерик). (3) Tool-side: edit отдаёт в Output только дифф (model-facing notice остаётся в Content), Detail — чистый путь (и в DetailFromArgs убран «--edits N»); write диффует против прежнего содержимого (новый файл — сплошные +) и кладёт дифф в Output; read post-run Detail — «file (N lines)» / «(lines A-B of N)» / «(8.0 MiB)» / «(empty)»; grep — «"pat" — N matches in M files» с клипом длинного паттерна (grepDetail + тесты); find — «"glob" — N files»; ls — «dir (N entries)»; mcp inspect/call — «server · tool». (4) Ошибки не прячутся за разворотом: свёрнутый ToolBlock показывает первую строку Error, упавший BashBlock — последнюю непустую строку вывода, DiffBlock — ошибку под заголовком (block-тесты diff_block_test.go, 6 шт). Раздел «The transcript feed» добавлен в DESIGN.md, запись в CHANGELOG (конфликт с planless-веткой разрешён объединением). Гейт rc=0, happ-диагностика чистая, main `make test` rc=0.

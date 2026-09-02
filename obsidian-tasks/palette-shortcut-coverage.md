---
id: palette-shortcut-coverage
title: Command palette shortcuts filled from the keys catalog
status: done
tags:
    - ui
    - ux-standard
    - polish
created_at: "2026-09-01T11:15:10.000000Z"
updated_at: "2026-09-01T13:05:00.000000Z"
---

## Body

**Проблема.** Палитра умеет рисовать `cmd.Shortcut`, но заполнен он у трёх команд из двадцати одной (`internal/tui/commands/builtins.go`). Команда с хоткеем, не показывающая его в палитре, не учит пользователя быстрому пути — палитра остаётся единственным способом навсегда.

**Что сделать.** Для каждой builtin-команды, у которой есть глобальный хоткей (settings — Ctrl+,, plan — Ctrl+P, help — F1, context, connect и т.д.), заполнить `Shortcut` — в идеале не строкой, а ссылкой на биндинг каталога `internal/tui/keys`, чтобы палитра и футер не могли разойтись. Если [[key-binding-table]] сделан первым — брать из таблицы биндингов автоматически.

**Критерий.** Каждая команда палитры, достижимая хоткеем, показывает его; тест сверяет `Shortcut` палитры с каталогом keys.

**Результат.** Сделано (commit 9c55bd3, merge 433f822). Аккорды вынесены в константы `keys.Chord*`, каталог и палитра ссылаются на одни и те же; slash-команды help/context/connect/compact/sessions получили строки в Ctrl+K-палитре (help — с F1). Тест `TestPaletteShortcutsComeFromTheKeysCatalog` сверяет каждый `Shortcut` палитры с каталогом. Полная конфигурируемая таблица биндингов остаётся в [[key-binding-table]].

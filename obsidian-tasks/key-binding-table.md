---
id: key-binding-table
title: Binding table over the keys catalog; configurable keybinds
status: done
tags:
    - ui
    - ux-standard
    - debt
created_at: "2026-09-01T11:15:10.000000Z"
updated_at: "2026-09-01T17:40:00.000000Z"
---

## Body

**Проблема.** `Editor.Handle` (`internal/tui/editor/editor.go:613` и дальше) — лестница из полутора десятков `if`, где владение клавишей задаётся порядком веток; в комментариях описаны прошлые баги фокуса («used to steal focus», stale planFocus). Каждый новый хоткей — новый `if`: его нельзя переназначить, нельзя проверить на конфликт, он ломается от перестановки веток. Биндинги не конфигурируются вообще — слов `keybind`/`keymap` в конфиге нет. Симптом той же болезни: `Ctrl+P` (панель плана, :681) и `Alt+P` (фокус плана в сайдбаре, :672) — два соседних сочетания на две разные вещи.

**Что уже есть.** Каталог `internal/tui/keys` — единственный источник футеров и `/help`; это готовый субстрат: у каждого биндинга есть scope, label, desc.

**Что сделать.** (1) Таблица биндингов поверх каталога: chord → command id в рамках scope, диспатч `Editor.Handle` по таблице вместо лестницы; порядок веток заменяется явной моделью фокуса/приоритета scope-ов. (2) Секция `keybinds` в конфиге: переопределение chord для command id, с валидацией конфликтов при загрузке (дубль в одном scope — ошибка). (3) `/help` и футеры автоматически показывают переопределённые сочетания — они уже рендерятся из каталога. (4) Пересмотреть пару Ctrl+P/Alt+P.

**Критерий.** Ни одного захардкоженного chord-сравнения в `Editor.Handle`; переопределение в конфиге меняет и поведение, и футер, и `/help`; тест на конфликт двух команд на одном chord. Связано: [[palette-shortcut-coverage]].

**Результат.** Сделано (commit 46cd9ee, merge в main 8f83182). (1) В `internal/tui/keys` появились `chord.go` (тип `Chord`: парсер спеллингов «Ctrl+P»/«F1»/«Cmd+C», каноническая печать, точное сравнение модификаторов — Ctrl+P и Ctrl+Shift+P разные аккорды, иначе проверка конфликтов бессмысленна) и `table.go` (таблица command id → chord с дефолтами, повторяющими прежние захардкоженные сочетания: help/palette/settings/plan-editor/plan-focus/sidebar-toggle/plan-approve/plan-details/copy-last). (2) `Editor.Handle` диспатчит через `keys.GlobalCommand` + `runGlobalCommand`; в editor.go не осталось НИ ОДНОГО сравнения аккорда (grep по HotkeyRune/KeyF1/ModCtrl/ModAlt пуст); sidebar отдал chord-свободные действия `ToggleVisibility`/`TogglePlanApproved`/`TogglePlanDetails`, transcript — `CopySelectionOrLast`, composer матчит палитру через `keys.Is`. Неприменимая сейчас команда (Ctrl+A без плана) возвращает false и падает дальше по лестнице — аккорд не глохнет. (3) Секция `keybinds` в конфиге: переопределение per id, `none` снимает биндинг, запятая — синонимы; валидация в `parseConfigFile` через `keys.CheckBinds` (неизвестный id, кривой спеллинг, две команды на одном chord — ошибка загрузки с именами обеих команд), применение `keys.Rebind` в runTUI до постройки UI. (4) Каталог ссылается на command id (`Binding.Cmd`, `Group.TitleCmd`), `Groups()/Find()` материализуют текущие спеллинги — футеры, `/help` и Shortcut-колонка палитры меняются вместе с поведением одним переопределением; отвязанная команда исчезает из подсказок. (5) Пара Ctrl+P/Alt+P пересмотрена и оставлена: теперь это два явных id (`plan-editor`/`plan-focus`) с разными описаниями в help, каждый переназначаем — решение записано в DESIGN.md. Тесты: chord_test (грамматика/каноникализация/точность матча), table_test (конфликт двух команд на одном chord с именами обеих, unknown id, none, обновление Label/Hints/Find после Rebind), keybinds_test в project (валидация при загрузке), keybind_test в editor (переопределение реально меняет диспатч Editor.Handle). Контракт — в DESIGN.md («Chords and the binding table») и CHANGELOG.

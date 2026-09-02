---
status: done
created_at: "2026-08-25T15:20:00.000000Z"
resolved_by: daa08bd chore: merge composer-editor into main
---

# composer-editor

## Body

Поле ввода не соответствовало базовым ожиданиям текстового редактора: нельзя
было выделить и скопировать текст, Ctrl+C всегда выходил, wrap резал слова
посреди буквы, Up/Down/Home/End не знали о визуальных строках.

## Solution

Один глубокий модуль внутри `chat.ChatInput` — `layoutEditor` (word-wrap
раскладка с кластер/ширина/offset маппингом) является единственным источником
истины: Draw рисует его строки, каретка/мышь/selection отображаются через
него. Routing: `components.CopyKeyAcceptor` — App спрашивает focused-виджет
до встроенного выхода по Ctrl+C.

## Acceptance Criteria

- [x] Клик ставит каретку; drag выделяет; hover без кнопки не мутирует
- [x] Shift+Left/Right/Home/End/Up/Down расширяют выделение от якоря
- [x] Ctrl/Cmd+A — выделить всё
- [x] Ctrl+C / Ctrl+Shift+C / Cmd+C копируют поле при выделении; без
      выделения поведение прежнее (выход / transcript)
- [x] Ctrl+X вырезает; ввод и Paste печатают поверх выделения
- [x] Backspace/Delete удаляют выделение целиком
- [x] Wrap по словам; длинные токены hard-wrap
- [x] Up/Down/Home/End по визуальным строкам
- [x] Ctrl/Alt+Left/Right/Backspace/Delete — word-варианты
- [x] CJK: клик/каретка не попадают в середину широкого глифа

## Review findings (исправлены до мержа)

- Ctrl+Shift+C выходил из приложения (xui CtrlC() не исключает Shift) —
  AcceptCopyKey теперь не фильтрует Shift, единый isChord
- Залипающий drag мутировал каретку при hover (?1003 all-motion)
- Unconsumed mouse ре-диспетчеризовался в focused-виджет с абсолютными
  координатами — dispatch теперь не отдаёт мышь focused-виджету
- Фантомная свёрнутая selection глотала первый Backspace
- End на короткой свёрнутой строке рисовал каретку строкой ниже
- Copy-failure теперь показывает toast, а не молчит

## Residual (осознанно)

- Press-capture мыши не реализован: drag, начатый в transcript и прошедший
  над композером с заломленным dragging, может подвинуть каретку (редкий
  кейс, требует capture-семантики в App).
- moveVisual не помнит goal column (колонка съезжает через короткую строку).

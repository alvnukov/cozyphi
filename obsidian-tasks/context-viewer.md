---
status: done
created_at: "2026-08-26T10:00:00.000000Z"
resolved_by: merge of feat/context-viewer (d43080c, 84e6e41, 4c6960e)
---

# context-viewer

## Body

Просмотр контекста (/context) показывал только однострочные превью, Enter
закрывал браузер, а единственная операция — trim «всё до записи». Нужно:
просмотр полного содержимого блока во всплывающем окне, удаление блока,
Shift-выделение нескольких блоков и их удаление.

## Solution

- **session**: `Compaction.DroppedEntryIDs` — маска удалённых записей;
  `buildSessionContext` фильтрует по ней; `appendCompactionLocked`
  (общий для trim/auto-compact/drop) наследует маску и объединяет с новой;
  `DropContextEntries` валидирует id, встаёт якорем на первую message-запись
  текущего контекста и подшивает предыдущий summary в маркер удаления.
- **ContextItem.Body**: полный текст блока для popup; Preview = previewLine(Body).
- **ctxpane**: Enter — popup с телом блока (рамка, j/k/стрелки/PgUp/PgDn/wheel,
  Enter/Esc/q закрыть, все клавиши потребляются); Shift+Up/Down — диапазон
  (якорь при первом сдвиге, обычные ходы и gg/G схлопывают); Delete/Backspace/d —
  подтверждение y/n, удаление выбранных блоков одним вызовом seam onDelete.
- **wiring**: engine/controller `DropContextEntries` c busy-guard, тосты.

## Acceptance Criteria

- [x] Enter открывает popup с полным содержимым; скролл; Enter/Esc закрывают
- [x] Popup потребляет все клавиши (список под ним не реагирует)
- [x] Delete/Backspace/d удаляет блок после y/n
- [x] Shift+Up/Down — диапазон в обе стороны; Delete удаляет все выбранные
- [x] Удалённые блоки исчезают из BuildContext и браузера
- [x] Trim и повторные drop не воскрешают удалённые блоки (наследование маски)
- [x] Drop после компакции сохраняет предыдущий summary в маркере
- [x] Summary-строку удалить нельзя; busy-guard при ответе модели
- [x] Подтверждения trim/delete взаимоисключающие

## Review findings (исправлены до мержа, 4c6960e)

- Блокер: DropContextEntries обходил inherit-union — вторая drop в обратном
  порядке воскрешала первую; теперь через appendCompactionLocked
- Drop после компакции выбрасывал summarized history — summary подшивается
- Двойной 'y' мог удалить и затем trim — подтверждения взаимоисключающие
- gg/G/Shift+G не схлопывали range — массовое удаление по случайному якорю
- Popup End: последняя видимая строка внизу, а не последняя строка вверху

## Residual (осознанно)

- Валидация drop по всему дереву (byIDs), а не по текущему пути — та же
  строгость, что у trim; недостижимо из UI.
- Старый бинарл, открыв сессию с droppedEntryIds и перезаписав её, потеряет
  маску (schema growth; omitempty).
- Мышь в списке/popup: только wheel; клики-переходы по строкам — отдельная
  задача.

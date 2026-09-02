---
id: refactor-delete-dead-code
title: 'Мёртвый код: layout-zoo ~700 строк, второй текст-редактор, хвосты'
status: todo
priority: medium
task_type: refactor
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - layout-zoo удалён (live-хелперы остаются)
    - один текст-редактор с общей санитизацией
    - перечисленные хвосты удалены
    - doc/tui.md обновлён
verification_plan:
    - make lint test
created_at: "2026-08-23T15:17:22.124309Z"
updated_at: "2026-08-23T15:17:22.124309Z"
---

## Body

layout.go (856 строк, крупнейший файл) — 13 widget-типов с нулём пользователей вне пакета (Padding, SizedBox, Spacer, Flexible, Divider, Stack, Positioned, Clickable, Container, Text, Button, Box, EdgeInsets; live только BorderLabel/DrawRoundedBorder/TruncateToWidth/BorderRounded). input.TextField (input.go:37-99) — вторая реализация текст-редактирования, insert без санитизации, которую chat_input.go:310-343 документирует как необходимую против десинка ячеек. Далее: Bus.Chan (bus.go:124-129) ноль вызовов, composer.PaletteComposer мёртв, preferredAskHeight мёртвые параметры (overlays.go:656-657), job.HandleSpawn+SpawnArgs (tools.go:13-53), Manager.GetBranch только тесты, Depth/MaxDepth гипотетический шов (всегда 0/1), SetModel всегда-nil error, truncate переизобретён 5 раз, doc/tui.md врёт про RequestRedraw->QueueRefresh и 'thin Editor root'.

## Acceptance Criteria

- layout-zoo удалён (live-хелперы остаются)
- один текст-редактор с общей санитизацией
- перечисленные хвосты удалены
- doc/tui.md обновлён

## Verification Plan

1. make lint test

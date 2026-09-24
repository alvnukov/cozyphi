---
id: watch-reminder-unescaped-close-tag
title: 'WatchReminder не экранирует </system-reminder> в выводе watch-команды'
status: todo
priority: medium
model_level: low
task_type: bug
tags:
    - watch
    - security
    - prompt
acceptance_criteria:
    - 'Вывод watch-события с `</system-reminder>` внутри не закрывает рамку раньше времени: WatchReminder экранирует закрывающий тег так же, как agent.QueueSessionContext'
    - 'Тест рядом с watch.go: событие с `</system-reminder>` в Text даёт ровно один закрывающий тег — последний'
verification_plan:
    - go test ./internal/agent/ -run Watch
created_at: "2026-09-24T18:00:00Z"
updated_at: "2026-09-24T18:00:00Z"
---

## Body

**Что.** `WatchReminder` (`internal/agent/watch.go`) вставляет `ev.Text` в блок `<system-reminder>` как есть. Если вывод watch-команды содержит `</system-reminder>`, рамка закрывается посреди события: остаток вывода модель читает уже вне рамки «это не инструкция пользователя». К тому же `memory.StripReminders` при повторном проигрывании транскрипта режет блок не там, где он кончается.

**Откуда.** Найдено при финальном ревью ветки `feature/claude-plugins`. Там тот же дефект исправлен для контекста плагинов (`QueueSessionContext` заменяет `</system-reminder>` на `<\/system-reminder>`). Дефект в watch.go был до этой ветки, и в её скоуп он не вошёл.

**Как чинить.** Вынести экранирование в одно место, которым пользуются и `QueueSessionContext`, и `WatchReminder`. Проверить, нужно ли то же для `</watch>` внутри события.

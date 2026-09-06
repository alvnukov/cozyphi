---
id: z.ai-monthly-window-humanize-minute-counter
title: 'z.ai: show available limit resets and expiry'
status: done
priority: medium
task_type: bug
branch: bug/z.ai-monthly-window-humanize-minute-counter
worktree_path: .worktrees/z.ai-monthly-window-humanize-minute-counter
created_at: "2026-09-06T12:21:37.815008Z"
updated_at: "2026-09-06T13:49:16.339247Z"
---

## Body

User clarified the real requirement (2026-09-06): TIME_LIMIT in /api/monitor/usage/quota/limit is the MANUAL LIMIT RESET counter — the z.ai dashboard button spends one reset. Fields: usage=granted, currentValue=spent, remaining=available, nextResetTime=when the resets expire. Show "N available" + expiry date next to the weekly and 5-hour resets in status pane, /usage, sidebar. The earlier "monthly usage-duration minutes" reading was wrong; merge 4b4b095's "0 / 1000 min" row must be replaced, no durations anywhere. Plan rev 125 approved.

**Done (2026-09-06).** TIME_LIMIT z.ai декодируется как кредиты сбросов лимита: квота-строка (unit "resets") показывает число доступных сбросов и дату протухания рядом с недельным и 5-часовым окнами. Панель /usage: "1000 available" + дата; сайдбар: строка "N available" + отдельная строка "expires <дата>". Коммит 080ac60, merge b16e86c в main. Счётчиков минут не осталось.

**Reopened (2026-09-06).** Пользователь отклонил текущий результат: реализация не соответствует запросу. Возвращаю задачу на диагностику фактического API payload и исправление UX.

**Started (2026-09-06).** Повторная работа после отклонения UX: сначала проверяю фактический TIME_LIMIT payload и точный текущий рендер, затем исправляю без догадок.

**Done (2026-09-06).** Исправление слито в main: TIME_LIMIT моделируется как доступные ручные сбросы с expiry, а не usage-window; /usage и sidebar показывают одинаковые явные строки рядом с 5-hour/week. Scoped tests/build прошли; единственный scoped lint сообщил только предсуществующий unparam в нетронутом sidebar_plan_test.go.

**Done (2026-09-06).** По живому payload доказано: TIME_LIMIT — месячный бюджет инструментов (usage=выдано, currentValue=потрачено, usageDetails: search-prime/web-reader/zread), полей ручных сбросов в API нет. Выдуманная строка «limit resets N available» убрана; TIME_LIMIT рендерится обычным окном 0/1000 + reset date сразу после 5h/week; ExpiresAt удалён из QuotaResetSummary (никто не заполняет). Слито в main (e4ea5e2, merge).

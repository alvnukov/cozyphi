---
id: z.ai-monthly-window-humanize-minute-counter
title: 'z.ai: show available limit resets and expiry'
status: done
priority: medium
task_type: bug
branch: bug/z.ai-monthly-window-humanize-minute-counter
worktree_path: .worktrees/z.ai-monthly-window-humanize-minute-counter
created_at: "2026-09-06T12:21:37.815008Z"
updated_at: "2026-09-06T12:53:20.699786Z"
---

## Body

User clarified the real requirement (2026-09-06): TIME_LIMIT in /api/monitor/usage/quota/limit is the MANUAL LIMIT RESET counter — the z.ai dashboard button spends one reset. Fields: usage=granted, currentValue=spent, remaining=available, nextResetTime=when the resets expire. Show "N available" + expiry date next to the weekly and 5-hour resets in status pane, /usage, sidebar. The earlier "monthly usage-duration minutes" reading was wrong; merge 4b4b095's "0 / 1000 min" row must be replaced, no durations anywhere. Plan rev 125 approved.

**Done (2026-09-06).** TIME_LIMIT z.ai декодируется как кредиты сбросов лимита: квота-строка (unit "resets") показывает число доступных сбросов и дату протухания рядом с недельным и 5-часовым окнами. Панель /usage: "1000 available" + дата; сайдбар: строка "N available" + отдельная строка "expires <дата>". Коммит 080ac60, merge b16e86c в main. Счётчиков минут не осталось.

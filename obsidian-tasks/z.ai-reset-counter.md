---
id: z.ai-reset-counter
title: 'Z.AI: отобразить общий ресет подписки (счётчик из zcode-семантики)'
status: done
priority: medium
task_type: feature
branch: feature/z.ai-reset-counter
worktree_path: .worktrees/z.ai-reset-counter
verification_plan:
    - Сырьцы zcode найдены, endpoint/поле общего ресета названы с сырым JSON-свидетельством (ключ редacted)
    - Если endpoint жив — живой захват через curl с тем же ключом
    - Регрессионный тест на новой форме ответа (красный→зелёный) в internal/provider
    - Гейты по изменённым пакетам, CHANGELOG [Unreleased], merge --no-ff, ledger, worktree cleanup
created_at: "2026-09-06T11:57:57.131579Z"
updated_at: "2026-09-06T12:11:11.508083Z"
---

## Body

Пользователь на стороне z.ai имеет «общий» ресет (один) — не 5-часовой/недельный оконный. В ответе /api/monitor/usage/quota/limit такого счётчика нет. Официальный клиент z.ai (zcode) открыт — найти в его сырцах, откуда берётся счётчик ресетов (endpoint, поле, семантика), затем отобразить его в квотных поверхностях cozyphi (статус-пан, /usage, сайдбар) рядом с существующими ResetsAt окон.

Контекст: только что закрыта задача z.ai (нули квоты после дрейфа 2026-09-06, коммит c8f0b06, merge 76b6fe5) — процентные окна декодируются, ResetsAt рендерится. Этот ресет — отдельная сущность.

**Done (2026-09-06).** Общий (месячный) ресет z.ai найден и посажен: TIME_LIMIT в /api/monitor/usage/quota/limit — это бюджет usage-длительности (usage=грант, currentValue=потрачено, nextResetTime=общий ресет), юнит 5 = месяц. Источник семантики: открытая экосистема zcode (zcode-switch src-tauri/src/quota.rs). Код: internal/provider/quota.go (zaiLimitAmounts CREDIT+TIME, zaiWindow unit5=month/unit4=day), internal/tui/usagepane/pane.go (мин-ветка). Тест TestQuotaSnapshotZAIPercentageOnlyLimits закрепляет живую форму. Merge 4b4b095 в main.

## Verification Plan

1. Сырьцы zcode найдены, endpoint/поле общего ресета названы с сырым JSON-свидетельством (ключ редacted)
2. Если endpoint жив — живой захват через curl с тем же ключом
3. Регрессионный тест на новой форме ответа (красный→зелёный) в internal/provider
4. Гейты по изменённым пакетам, CHANGELOG [Unreleased], merge --no-ff, ledger, worktree cleanup

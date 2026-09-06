---
id: z.ai-reset-counter
title: 'Z.AI: отобразить общий ресет подписки (счётчик из zcode-семантики)'
status: in_progress
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
updated_at: "2026-09-06T11:58:21.89045Z"
---

## Body

Пользователь на стороне z.ai имеет «общий» ресет (один) — не 5-часовой/недельный оконный. В ответе /api/monitor/usage/quota/limit такого счётчика нет. Официальный клиент z.ai (zcode) открыт — найти в его сырцах, откуда берётся счётчик ресетов (endpoint, поле, семантика), затем отобразить его в квотных поверхностях cozyphi (статус-пан, /usage, сайдбар) рядом с существующими ResetsAt окон.

Контекст: только что закрыта задача z.ai (нули квоты после дрейфа 2026-09-06, коммит c8f0b06, merge 76b6fe5) — процентные окна декодируются, ResetsAt рендерится. Этот ресет — отдельная сущность.

## Verification Plan

1. Сырьцы zcode найдены, endpoint/поле общего ресета названы с сырым JSON-свидетельством (ключ редacted)
2. Если endpoint жив — живой захват через curl с тем же ключом
3. Регрессионный тест на новой форме ответа (красный→зелёный) в internal/provider
4. Гейты по изменённым пакетам, CHANGELOG [Unreleased], merge --no-ff, ledger, worktree cleanup

---
id: startup-last-used-model
title: Старт новой сессии на последней использованной модели
status: done
priority: high
task_type: feature
tags:
    - tui
    - session
    - model
created_at: "2026-08-31T08:38:39.893388Z"
updated_at: "2026-08-31T09:03:44.587138Z"
---

## Body

**Задача:** при свежем старте TUI новая сессия открывается на дефолтной модели конфига. Нужно запоминать последнюю использованную модель и стартовать с неё.

**Решение:** имя модели в глобальный ui.json (project.UIState.LastModel), резолв при старте через Controller.findModel, COZYPHI_MODEL перекрывает, устаревшее имя откатывается к дефолту; сохранение на старте/SetModel/Resume.

**Вне скоупа:** headless cozyphi run.

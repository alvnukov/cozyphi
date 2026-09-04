---
id: model-effort-display-gaps
title: Бейдж модели шага в сайдбаре теряет effort сессии
status: done
priority: high
created_at: "2026-09-04T23:23:22.117636Z"
updated_at: "2026-09-04T23:35:43.964785Z"
---

## Body

Найдено при живой проверке отображения после задачи unify-model-effort-picker.

**Баг 1 (сайдбар):** бейдж модели шага (`stepModelBadge`, internal/tui/sidebar/sidebar.go:1514) для шага без пина показывает голое имя — editor.go:222 и :1016 кладут в `sidebar.Runtime.SessionModel` результат `ctrl.ModelName()` (голое имя из modelCfg.Name), а не реф с effort. Двигатель гоняет незакреплённые шаги на сессионной модели с её effort — бейдж должен показывать `name · effort`.

**Баг 2 (план-редактор):** секция Step models в browse-виде (internal/tui/planedit/pane.go:2886) рендерит сырой реф `name:effort` вместо `name · effort`.

Фикс: `Controller.ModelRef()` через `session.FormatModelRef(configuredModelName(), configuredEffort())`; editor передаёт его в SessionModel; planedit рендерит через `session.ModelLabel`. Тесты: sidebar-бейдж с рефом и без, ModelRef, planedit Step models.

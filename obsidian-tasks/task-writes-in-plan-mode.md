---
id: task-writes-in-plan-mode
title: 'Настройка permissions.tasks: человек решает, как модель работает с реестром задач (off / read / ask / write, во всех режимах)'
status: done
priority: high
model_level: high
task_type: feature
parent_id: native-task-tool
tags:
    - tools
    - tasks
    - permission
    - settings
    - plan
    - prompt
acceptance_criteria:
    - Ключ permissions.tasks со значениями off / read / ask / write (по умолчанию write) читается из ~/.cozyphi/config.yaml, неверное значение — ошибка загрузки с перечнем допустимых.
    - 'Gate: task_read Allow при любом уровне кроме off; task_write — Deny с объяснением при read, Ask при ask, Allow при write. task_write не считается мутацией: в ModeReadonly (plan-оверлей) уровень действует так же, и Ask для task_write доходит до человека, а не сворачивается в Deny; в autopilot/headless Ask сворачивается как раньше.'
    - Тула task при off не регистрируется и абзац промпта отсутствует; при read схема предлагает только current/list/get, а абзац говорит описывать изменения для пользователя; при ask абзац просит одно полное изменение вместо нескольких; plan-аппендикс говорит, что формирование задач — часть планирования, а start ждёт исполнения.
    - Вкладка General настроек TUI содержит строку уровня доступа к реестру; выбор сохраняется в permissions.tasks глобального конфига и применяется к текущей сессии без перезапуска (gate и набор тул).
    - doc/tasks.md описывает ключ и таблицу уровней; пункт CHANGELOG в Unreleased описывает итоговое поведение; make fmt-check lint test rc=0.
verification_plan:
    - go test ./internal/permission/... ./internal/project/... ./internal/tools/tasktool/... ./internal/agent/... ./internal/harnesssettings/... ./internal/tui/settings/... -race
    - make fmt-check lint test
created_at: "2026-09-03T05:13:39.322902Z"
updated_at: "2026-09-03T05:56:29.784158Z"
---

## Body

**Проблема.** В native-task-tool запись реестра объявлена мутацией, и plan-оверлей (ModeReadonly) её отбивает. Но plan-режим — место, где задачи формулируются; при этом одни пользователи хотят, чтобы модель вела реестр сама, другие — подтверждать каждое изменение или запретить запись вовсе. Единого правильного ответа нет, решать должен человек, как он решает про bash, mcp и workspace-only.

**Решение (согласовано 2026-09-03).** Одна настройка `permissions.tasks` со значениями off / read / ask / write, по умолчанию write. Уровень действует одинаково во всех режимах, включая plan: off — тулы нет и абзаца в промпте нет; read — в схеме только current/list/get, запись отказана с объяснением; ask — запись спрашивает человека, в том числе в plan-режиме (исключение из свёртки Ask→Deny для task_write: реестр не код, а человек рядом и сам выбрал ask); write — без вопросов, как запись памяти. task_write выводится из isMutating. Абзац промпта подстраивается под уровень, plan-аппендикс поясняет, что формирование задач — часть планирования, а start ждёт исполнения. Строка на вкладке General настроек TUI сохраняет выбор в глобальный конфиг и применяет к текущей сессии (gate + набор тул). Документация: doc/tasks.md, правка существующего пункта CHANGELOG.

**Итог (2026-09-03).** Реализовано и влито в main: merge 15700b7 (коммит функции cc6458d). Тип уровня `tasks.Access` живёт в internal/tasks (ParseAccess, Normalized, Writable, Next); `Policy.Tasks` в internal/permission, gate: task_read Allow (Deny при off), task_write Allow / Ask / Deny по уровню, task_write выведен из isMutating, в ModeReadonly Ask для task_write не сворачивается; конфиг `permissions.tasks` с ошибкой на неизвестное значение; тула task при off не регистрируется, при read предлагает только current/list/get и отказывает записи с подсказкой; абзац системного промпта на три варианта и plan-аппендикс для writable-уровней; Engine.SetTasksAccess и Controller.SetTasksAccess применяют уровень вживую; harnesssettings читает и пишет permissions.tasks, строка «Task registry access» на вкладке General циклит write → ask → read → off; редактор применяет снапшот к контроллеру. doc/tasks.md (раздел Permission) и пункт CHANGELOG обновлены. Гейт make fmt-check lint test rc=0.

## Acceptance Criteria

- Ключ permissions.tasks со значениями off / read / ask / write (по умолчанию write) читается из ~/.cozyphi/config.yaml, неверное значение — ошибка загрузки с перечнем допустимых.
- Gate: task_read Allow при любом уровне кроме off; task_write — Deny с объяснением при read, Ask при ask, Allow при write. task_write не считается мутацией: в ModeReadonly (plan-оверлей) уровень действует так же, и Ask для task_write доходит до человека, а не сворачивается в Deny; в autopilot/headless Ask сворачивается как раньше.
- Тула task при off не регистрируется и абзац промпта отсутствует; при read схема предлагает только current/list/get, а абзац говорит описывать изменения для пользователя; при ask абзац просит одно полное изменение вместо нескольких; plan-аппендикс говорит, что формирование задач — часть планирования, а start ждёт исполнения.
- Вкладка General настроек TUI содержит строку уровня доступа к реестру; выбор сохраняется в permissions.tasks глобального конфига и применяется к текущей сессии без перезапуска (gate и набор тул).
- doc/tasks.md описывает ключ и таблицу уровней; пункт CHANGELOG в Unreleased описывает итоговое поведение; make fmt-check lint test rc=0.

## Verification Plan

1. go test ./internal/permission/... ./internal/project/... ./internal/tools/tasktool/... ./internal/agent/... ./internal/harnesssettings/... ./internal/tui/settings/... -race
2. make fmt-check lint test

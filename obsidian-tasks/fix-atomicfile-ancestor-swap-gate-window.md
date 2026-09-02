---
id: fix-atomicfile-ancestor-swap-gate-window
title: 'atomicfile: подмена родительского каталога между gate и записью проходит'
status: done
priority: high
task_type: bug
parent_id: permission-symlink-workspace-escape
tags:
    - security
    - symlink
    - toctou
    - review-2026-09
acceptance_criteria:
    - Подмена любого родительского каталога симлинком между одобрением gate и rename прерывает запись с ошибкой, называющей путь
    - Комментарий пакета atomicfile, write.go и CHANGELOG описывают фактическую защиту без переоценки
    - edit-путь (hashline) получает ту же защиту
verification_plan:
    - 'Детерминированный тест: gate одобряет путь, фикстура подменяет родителя симлинком до вызова atomicfile.Write, внешний файл не создаётся'
    - go test ./internal/atomicfile/ ./internal/tools/writetool/ ./internal/permission/
created_at: "2026-09-02T16:51:43.320933Z"
updated_at: "2026-09-02T19:14:39.514Z"
---

## Body

**Проблема.** `internal/atomicfile/atomicfile.go:58` снимает якорь `stageDir` через `EvalSymlinks(dir)` уже внутри `write()`, а `os.MkdirAll(dir)` перед ним следует по симлинку. Перепроверка перед rename сравнивает каталог с этим же якорем, поэтому подмена родителя между одобрением gate и запуском инструмента не замечается. `runWrite` (`internal/tools/writetool/write.go:80`) ничего не перепроверяет.

**Сценарий.** gate одобрил `ws/a/b/f.txt`; пока висит ask, `ws/a` переименовывается и на его месте появляется симлинк на `/outside`; `atomicfile.Write` создаёт `/outside/b` и кладёт файл в `/outside/b/f.txt`. Комментарий пакета, тело коммита f8b40ad и CHANGELOG утверждают, что такой случай прерывает запись.

**Как чинить.** Передавать в мутацию цель, резолвнутую gate (или заново прогонять `permission.ResolveTarget` и проверку принадлежности workspace непосредственно перед rename), либо сверять `stageDir` с резолвнутым корнем workspace. Доки и CHANGELOG привести к реальному поведению. Найдено ревью правок после v0.19.0.

## Acceptance Criteria

- Подмена любого родительского каталога симлинком между одобрением gate и rename прерывает запись с ошибкой, называющей путь
- Комментарий пакета atomicfile, write.go и CHANGELOG описывают фактическую защиту без переоценки
- edit-путь (hashline) получает ту же защиту

## Verification Plan

1. Детерминированный тест: gate одобряет путь, фикстура подменяет родителя симлинком до вызова atomicfile.Write, внешний файл не создаётся
2. go test ./internal/atomicfile/ ./internal/tools/writetool/ ./internal/permission/

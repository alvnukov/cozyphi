---
id: ui-draw-on-demand-render-pipeline
title: Draw-on-demand рендер и фикс дрожания курсора
status: done
priority: high
task_type: refactor
parent_id: cozyphi-convenience-program
tags:
    - ui
    - render
    - xui
acceptance_criteria:
    - make fmt и make lint без замечаний
    - go test ./... зелёный
    - 'простой TUI: 0 байт на tty и 0.0% CPU; анимация sphere ~7% CPU'
    - стриминг ответов и toast-таймеры работают без 60fps тикера
verification_plan:
    - make fmt-check && make lint && make test
    - 'tmux-проверка: capture-pane при простое, top при анимации'
created_at: "2026-08-23T14:45:57.930036Z"
updated_at: "2026-08-23T14:45:57.930036Z"
---

## Body

Завендорен xui (replace-директива), в renderer добавлен cursor diff cache и пропуск пустых кадров (0 байт при простое), планировщик в пакете app (min-merge, pacing floor), DrawContext.Wake/WakeIn/WakeAt, точки анимации sphere/spinner/toast, SIGCONT-перерисовка. Ветка feat/ui-render-pipeline, 9 коммитов от main (0a3e5e9).

## Acceptance Criteria

- make fmt и make lint без замечаний
- go test ./... зелёный
- простой TUI: 0 байт на tty и 0.0% CPU; анимация sphere ~7% CPU
- стриминг ответов и toast-таймеры работают без 60fps тикера

## Verification Plan

1. make fmt-check && make lint && make test
2. tmux-проверка: capture-pane при простое, top при анимации

---
id: ui-tab-modes-agents
title: Tab-режимы Build/Plan и выбор @агента
status: done
priority: medium
task_type: feature
parent_id: cozyphi-convenience-program
tags:
    - ui
    - modes
acceptance_criteria:
    - Tab переключает Build/Plan, режим виден в футере
    - Plan-режим не запускает write-тулы без подтверждения
    - '@агент подсказками дополняется в composer'
verification_plan:
    - 'tmux: переключение и видимость режима'
    - тесты политик тулов
created_at: "2026-08-23T14:45:57.932406Z"
updated_at: "2026-08-23T22:29:11.226174Z"
---

## Body

Переключение Tab-ом режима Build/Plan (разные системные промпты и permission-политики) и упоминание @агента в composer для делегирования подзадачи.

## Acceptance Criteria

- Tab переключает Build/Plan, режим виден в футере
- Plan-режим не запускает write-тулы без подтверждения
- @агент подсказками дополняется в composer

## Verification Plan

1. tmux: переключение и видимость режима
2. тесты политик тулов

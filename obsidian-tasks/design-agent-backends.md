---
id: design-agent-backends
title: Спроектировать общий интерфейс Codex app-server и Claude Agent SDK
status: done
priority: medium
task_type: docs
branch: codex/design-agent-backends
worktree_path: .worktrees/design-agent-backends
acceptance_criteria:
    - Определены общий интерфейс и различия адаптеров
    - Сохранены ограничения permission gate, hashline-edit и изоляции дочерних агентов
    - Указаны неподтверждённые возможности и проверяемые этапы реализации
verification_plan:
    - Сопоставить с текущими Engine/Controller/job.Runner
    - Проверить локальную схему Codex и официальную документацию Claude
    - Проверить ссылки и diff документации
created_at: "2026-09-05T10:09:39.669763Z"
updated_at: "2026-09-05T10:14:49.479397Z"
---

## Body

Подготовить проект общего модуля внешних агентных сессий: два адаптера, события, запросы пользователя, управление процессами, права и хранение. Уточнить подключение к TUI и существующему job.Runner. Только проектирование, без реализации и изменения поведения.

## Acceptance Criteria

- Определены общий интерфейс и различия адаптеров
- Сохранены ограничения permission gate, hashline-edit и изоляции дочерних агентов
- Указаны неподтверждённые возможности и проверяемые этапы реализации

## Verification Plan

1. Сопоставить с текущими Engine/Controller/job.Runner
2. Проверить локальную схему Codex и официальную документацию Claude
3. Проверить ссылки и diff документации

---
id: stop-message-session-title
title: Имя сессии в сообщении об остановке вместо «#1 main»
status: in_progress
priority: high
model_level: medium
task_type: feature
tags:
    - multisession
    - tui
branch: feature/stop-message-session-title
worktree_path: .worktrees/stop-message-session-title
acceptance_criteria:
    - Stop-сообщение харнесса несёт имя текущей сессии (title из session set_title), а не «#1 main»
    - Без установленного title — прежний дефолт (метка вкладки)
    - Тест на публичном интерфейсе покрывает оба случая
    - 'CHANGELOG: строка под ## [Unreleased]'
    - Слито в main (--no-ff), ledger закоммичен, worktree и ветка удалены, задача закрыта
verification_plan:
    - Исследованием найти сборку stop-сообщения и источник метки «#1 main»
    - 'Тест публичного интерфейса: тайтл установлен / не установлен'
    - 'Гейты по изменённым пакетам: gofmt -l, один golangci-lint run, go test'
    - 'Ручная проверка: set_title → остановка → в сообщении имя сессии'
created_at: "2026-09-06T13:49:13.383559Z"
updated_at: "2026-09-06T14:02:18.668485Z"
---

## Body

**Что**: при остановке харнесс посылает сообщение, в котором сессия названа дефолтной меткой вкладки «#1 main». Нужно подставлять имя сессии, которое модель установила сама через session set_title.

**Почему**: пользователь в текущей сессии: «нужно чтобы в системном сообщении было имя вкладки, которую модель ставит сама, а не #1 main», уточнение: «при остановке харнесс посылает сообщение, вот в этом сообщении надо имя текущей сессии».

**Где**: точку сборки stop-сообщения найти исследованием (internal/tui, internal/tui/controller); имя сессии живёт рядом с session tool. Правку делать в одном глубоком месте, не в каждом вызове.

**Связанное**: multisession-background-attention (тосты/уведомления фоновых сессий) — там имя сессии тоже понадобится.

**Note (2026-09-06).** 2026-09-05: branch feature/stop-message-session-title, commit df6cfa3 "feat: name stop notifications by session title" (base 05ce564). Change: Controller.SessionName exposes the explicit session title (model set_title / user /rename); View.attentionOrigin makes SetIdentity push "#N <title>" to the notifier origin when a title exists, else the old "#N <slot>"; composer input line unchanged. Tests: attention_origin_test.go (both cases), scoped go test green; scoped golangci-lint shows only pre-existing issues in untouched files. CHANGELOG Unreleased line added. Pending: merge --no-ff into main, ledger commit, worktree/branch cleanup.

## Acceptance Criteria

- Stop-сообщение харнесса несёт имя текущей сессии (title из session set_title), а не «#1 main»
- Без установленного title — прежний дефолт (метка вкладки)
- Тест на публичном интерфейсе покрывает оба случая
- CHANGELOG: строка под ## [Unreleased]
- Слито в main (--no-ff), ledger закоммичен, worktree и ветка удалены, задача закрыта

## Verification Plan

1. Исследованием найти сборку stop-сообщения и источник метки «#1 main»
2. Тест публичного интерфейса: тайтл установлен / не установлен
3. Гейты по изменённым пакетам: gofmt -l, один golangci-lint run, go test
4. Ручная проверка: set_title → остановка → в сообщении имя сессии

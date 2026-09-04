---
id: go-version-badge
title: 'README: заменить мёртвый Go Report Card на бейдж версии Go из go.mod'
status: done
task_type: feature
branch: feature/go-version-badge
worktree_path: .worktrees/go-version-badge
acceptance_criteria:
    - README вместо retired-бейджа несёт shields-endpoint бейдж версии Go
    - CI на пуше в main публикует go.json (версия из go.mod) вместе с coverage.json на ветку badges
    - Ветка badges засеяна обоими JSON-файлами, оба URL отвечают 200
    - CHANGELOG-строка под [Unreleased] поправлена, conventional commit, main чист
verification_plan:
    - Локальная генерация обоих JSON и сверка версии с go.mod
    - curl raw URL обоих endpoint-JSON
    - git status основного checkout
created_at: "2026-09-04T21:59:54.772481Z"
updated_at: "2026-09-04T22:01:42.818519Z"
---

## Body

Go Report Card закрылся (бейдж «retired»). Замена: бейдж версии Go из go.mod через shields endpoint JSON на ветке badges — тот же механизм, что у coverage. CI пишет go.json рядом с coverage.json при пуше в main.

**Done (2026-09-05).** 2026-09-04: landed on main via feature/go-version-badge (commit bf2a9f8 "fix: replace retired Go Report Card badge with go.mod version"). Go Report Card закрылся — бейдж заменён на версию Go из go.mod через shields endpoint. CI coverage job теперь публикует два JSON (coverage.json + go.json, шаг переименован в "Publish badges"); ветка badges засеяна (d810ae3), оба raw URL отдают 200. Прямых живых аналогов Go Report Card нет (golangci.com — теперь только доксайт, goreport.io не существует, pkg.go.dev — бейдж документации, OpenSSF Scorecard — cozyphi ещё нет в датасете, добавить после ci-branch-protection). Локальные ветки-worktree артефакты сида (badges) чистятся сразу после push.

## Acceptance Criteria

- README вместо retired-бейджа несёт shields-endpoint бейдж версии Go
- CI на пуше в main публикует go.json (версия из go.mod) вместе с coverage.json на ветку badges
- Ветка badges засеяна обоими JSON-файлами, оба URL отвечают 200
- CHANGELOG-строка под [Unreleased] поправлена, conventional commit, main чист

## Verification Plan

1. Локальная генерация обоих JSON и сверка версии с go.mod
2. curl raw URL обоих endpoint-JSON
3. git status основного checkout

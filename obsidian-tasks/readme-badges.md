---
id: readme-badges
title: 'README: бейджи GitHub (CI, покрытие из репозитория, релиз, Go Report Card)'
status: done
task_type: feature
acceptance_criteria:
    - README несёт строку из 4 бейджей с реальным slug репозитория
    - Покрытие считается CI и публикуется endpoint-JSON на ветку badges без внешних сервисов и токенов
    - make cover воспроизводит локально; coverage.out игнорируется
    - CHANGELOG-строка под [Unreleased]; один conventional commit; main чист от кода
verification_plan:
    - make cover + scripts/coverage-badge.sh локально
    - curl всех 4 badge URL
    - Пуш ветки badges и повторный curl raw JSON
    - git status основного checkout
created_at: "2026-09-04T21:44:46.46679Z"
updated_at: "2026-09-04T21:44:51.016039Z"
---

## Body

Запрос: «сделай бейджи для гитхаба с покрытием тестами и другой полезной инфой». Покрытие — из репозитория (shields endpoint JSON на data-only ветке badges), без внешних сервисов. Бейджи: CI статус, покрытие, последний релиз, Go Report Card.

**Done (2026-09-05).** 2026-09-04: landed on main via feat/readme-badges (commit 499f353 "feat: add README badges with CI-published coverage"). README: строка из 4 бейджей (CI workflow, покрытие shields endpoint, github/v/release, Go Report Card). Plumbing: `make cover` (coverage.out в .gitignore), scripts/coverage-badge.sh → shields endpoint JSON, ci.yml coverage job публикует JSON force-push'ем одного orphan-коммита на ветку badges только на push в main (permissions: contents: write). Ветка badges засеяна локально (74.4%, yellow); все 4 URL отвечают 200. Гейты по Go-коду не гонялись — дифф не содержит .go-файлов; полный прогон делает CI.

## Acceptance Criteria

- README несёт строку из 4 бейджей с реальным slug репозитория
- Покрытие считается CI и публикуется endpoint-JSON на ветку badges без внешних сервисов и токенов
- make cover воспроизводит локально; coverage.out игнорируется
- CHANGELOG-строка под [Unreleased]; один conventional commit; main чист от кода

## Verification Plan

1. make cover + scripts/coverage-badge.sh локально
2. curl всех 4 badge URL
3. Пуш ветки badges и повторный curl raw JSON
4. git status основного checkout

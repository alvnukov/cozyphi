---
id: hooks-test-async-audit-marker
title: Тесты наблюдения хуков падали в CI — аудит-хук на * честно срабатывал после вызова harness
status: done
priority: high
model_level: medium
task_type: bug
parent_id: developer-mode-hooks
tags:
    - hooks
    - tests
    - ci
branch: fix/cozy-tools-require
worktree_path: .worktrees/cozy-tools-require
acceptance_criteria:
    - TestTheHeadlessRunReportsTheHooksItLoadedAndRunsNoneOfThem и TestTheIntegrationCatalogDeclaresTheHookKeysToo проходят детерминированно на ubuntu и macos в CI, не только на Маке разработчика.
    - Тест по-прежнему доказывает, что снимок и каталог harness не выполняют хуки: маркер не появляется, потому что ни один хук не подходит к вызванной туле.
verification_plan:
    - go test -race ./cmd -run 'TestTheHeadlessRun|TestAHeadlessRunWithHooks|TestTheIntegrationCatalog'; один прогон golangci-lint по cmd.
    - CI на PR fix/cozy-tools-require зелёный.
created_at: "2026-09-07T15:40:00Z"
updated_at: "2026-09-07T15:40:00Z"
---

## Body

**Симптом.** На PR #5 все три тестовых джоба (ubuntu, macos, coverage) падали в cmd: «asking about a hook is not what runs one» и «listing what can be asked for runs nothing at all» — файл-маркер, который пишут скрипты хуков, существовал. Локально на Маке тесты проходили (30 прогонов с -race, с TMPDIR как в CI, вне песочницы).

**Причина.** Фикстура installHooks объявляла `audit` как post_tool без match, то есть на `*`. Harness — обычная тула, и после её вызова менеджер честно запускает `audit` (async, отдельная горутина, context.Background). Проверка маркера идёт сразу после runHeadless, поэтому исход решала задержка запуска `/bin/sh`: на Маке первый запуск свежесозданного скрипта занимает ~330 мс (замер прямым вызовом Manager.PostTool), и маркер появлялся уже после assert; на раннерах CI он успевал раньше. Продукт ведёт себя правильно (doc/hooks.md: `*` = все тулы), неверно было утверждение теста.

**Исправление.** `audit` в фикстуре получает `match: bash`, как и `guard-bash`; run никогда не вызывает bash, поэтому отсутствие маркера доказывает ровно то, что нужно: снимок и каталог не выполняют хуки. Комментарий в фикстуре объясняет, почему хук на `*` здесь непригоден. Тест про выключенные хуки не менялся.

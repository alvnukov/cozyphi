---
id: permission-gate-no-leaf-stat-outside-home
title: Гейт не трогает системные чувствительные файлы при сборке — резолв через родителя
status: done
priority: medium
model_level: medium
task_type: bug
tags:
    - permission
    - security
branch: fix/permission-gate-no-leaf-stat
worktree_path: .worktrees/permission-gate-no-leaf-stat
acceptance_criteria:
    - При сборке гейта чувствительный префикс вне домашнего каталога (например /etc/shadow) резолвится через родительский каталог, имя файла приклеивается лексически; сам файл не получает ни lstat, ни EvalSymlinks.
    - Префиксы под домашним каталогом резолвятся полностью, как раньше, чтобы симлинк ~/.ssh на другой том по-прежнему перекрывался запретом.
    - Родитель системного пути по-прежнему резолвится (macOS /etc → /private/etc), так что совпадение с резолвнутыми целями сохраняется.
    - CHANGELOG Unreleased, тексты на английском.
verification_plan:
    - Тест — симлинк-лист вне home остаётся лексическим, симлинк-лист под home резолвится, родитель резолвится в обоих случаях.
    - go test -race ./internal/permission; go build ./cmd; один прогон golangci-lint по пакету.
created_at: "2026-09-07T14:10:00Z"
updated_at: "2026-09-07T14:10:00Z"
---

## Body

**Откуда.** Пользователь 2026-09-07 после permission-gate-windows-sensitive-paths: «это вообще-то файл с паролями» — коза не должна касаться /etc/shadow даже метаданными. Согласовано: резолвить только родителя для путей вне home.

**Done (2026-09-07).** Слито в main. `resolveSensitivePrefix(prefix, home)` в gate.go: под home — полный `ResolveTarget`, вне home — `ResolveTarget(parent)` + лексическое имя файла; `/etc/shadow` при сборке гейта больше не получает ни lstat, ни EvalSymlinks. Тест gate_sensitive_resolve_test.go на оба случая и на отсутствующий лист. Гейты: go test -race ./internal/permission, go build ./cmd (darwin и GOOS=windows), lint 0.

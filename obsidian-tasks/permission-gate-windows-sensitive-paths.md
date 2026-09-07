---
id: permission-gate-windows-sensitive-paths
title: Permission gate на Windows не собирается из-за /etc/shadow в списке чувствительных путей
status: done
priority: high
model_level: medium
task_type: bug
tags:
    - permission
    - windows
    - security
branch: fix/permission-gate-windows
worktree_path: .worktrees/permission-gate-windows
acceptance_criteria:
    - defaultSensitivePaths строится по ОС; на Windows в списке только пути под домашним каталогом, без /etc/shadow и /etc/passwd; на unix список прежний.
    - NewGate на Windows с DefaultPolicy собирается без ошибки (проверяется чистой функцией sensitivePathsFor(goos, home), чьи записи все абсолютны для данной ОС).
    - matchesPrefix на Windows сравнивает пути без учёта регистра, чтобы C:\Users\x\.ssh и c:\users\x\.ssh считались одним префиксом; на unix сравнение прежнее.
    - CHANGELOG Unreleased описывает починку; тексты на английском.
verification_plan:
    - Тесты в internal/permission на sensitivePathsFor для windows/linux/darwin и на регистронезависимое совпадение префикса под флагом.
    - go test -race ./internal/permission; go build ./cmd; GOOS=windows go vet ./internal/permission как кросс-проверка компиляции; один прогон golangci-lint по пакету.
    - Ручная проверка пользователем на Windows после пересборки: сессия стартует, запись в workspace разрешена, чтение %USERPROFILE%\.ssh отказано.
created_at: "2026-09-07T13:50:00Z"
updated_at: "2026-09-07T13:50:00Z"
---

## Body

**Откуда.** Пользователь 2026-09-07: «на винде вообще не работает пермишн гейт», в связке с вопросом, зачем гейт лезет в /etc/shadow.

**Причина.** `defaultSensitivePaths` (internal/permission/rules.go) безусловно добавляет `/etc/shadow`; `NewGate` резолвит каждый префикс через `ResolveTarget`, который требует `filepath.IsAbs`. На Windows путь без буквы диска не абсолютный, `NewGate` падает с «permission gate sensitive path "/etc/shadow": resolve target … is not an absolute path». В TUI и сконфигурированная политика, и `DefaultPolicy()` содержат тот же список, поэтому контроллер ставит `UnavailableGate` и всё запрещает; headless bootstrap и spawn детей возвращают ошибку.

**Попутно.** `matchesPrefix` сравнивает строки чувствительно к регистру, а файловая система Windows нет: отказ по `.ssh` обходился бы другим регистром буквы диска или каталога.

**Done (2026-09-07).** Слито в main. `sensitivePathsFor(goos, home)`: домашние записи везде, `/etc/shadow` (и fallback без home) только на unix; на Windows без home список пустой, чтобы NewGate не падал. `matchesPrefix` сравнивает через `samePath`, на Windows без учёта регистра (переменная `caseInsensitivePaths`, тесты покрывают оба режима). Гейты: go test -race ./internal/permission, go vet и go build ./cmd под GOOS=windows, lint 0. Нужна ручная проверка пользователем на Windows после пересборки.

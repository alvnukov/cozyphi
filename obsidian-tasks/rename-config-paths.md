---
id: rename-config-paths
title: 'Хвосты переименования: ~/.phi и phi update в CHANGELOG → cozyphi'
status: done
created_at: "2026-08-25T14:46:00.000000Z"
resolved_by: "d37e654 chore: merge rename-config-paths into main"
---

# rename-config-paths

## Body

Дожал переименование: последние `~/.phi`, `.phi/hooks` и `phi update` в
released-секции CHANGELOG.md заменены на `~/.cozyphi`, `.cozyphi/hooks`,
`cozyphi update`. Теперь в tracked-файлах нет ни `.phi`, ни голого `phi`
вне брендовых `cozyphi`/`CozyPhi`/`COZYPHI`.

## Acceptance Criteria

- [x] `git grep -F .phi` (без .git) — ноль вхождений
- [x] голый `phi` вне cozyphi/CozyPhi/COZYPHI — ноль вхождений
- [x] `go build ./...`, `go test ./...` зелёные

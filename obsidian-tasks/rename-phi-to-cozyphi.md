---
status: done
created_at: "2026-08-25T14:40:00.000000Z"
resolved_by: b51a4c6 chore: merge rename-phi-cozyphi into main
---

# rename phi to cozyphi

## Body

Полное переименование проекта: module path `github.com/alvnukov/cozyphi`,
бинарник `cozyphi`, env-префикс `COZYPHI_`, домашняя директория `~/.cozyphi`,
проектная `.cozyphi/`, брендинг `CozyPhi`, ассеты `cozyphi.png` /
`pixel-text-COZYPHI.png`.

Не тронуты: git-история, `xui/` (отдельный форк `pulseaiclub/xui`),
заблокированная released-секция CHANGELOG (CI protect-released-changelog).

## Acceptance Criteria

- [x] `git grep -i phi` (кроме xui/assets) не находит `phi` вне `cozyphi`/`CozyPhi`/`COZYPHI`
- [x] `go build ./...` и `go test ./...` зелёные
- [x] `make fmt`, `make fmt-check`, `make lint` чистые
- [x] released-секция CHANGELOG не изменилась

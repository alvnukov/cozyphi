---
id: fix-windows-build
title: Починить сборку под Windows и флейк в закрытии дочерней сессии
status: done
priority: high
tags:
    - ci
    - release
    - tests
branch: bug/fix-windows-build
worktree_path: .worktrees/fix-windows-build
created_at: "2026-09-06T00:00:00Z"
updated_at: "2026-09-06T00:00:00Z"
---

## Body

Продолжение [[fix-red-ci-main]]: после переноса тега прогон Release 34045124917 всё равно красный, и на b661a86 упала джоба `test (macos-latest)`. Два независимых дефекта.

**Windows не компилируется.** goreleaser собрал darwin и linux, а на `windows_amd64_v1` получил `internal/harnesssettings/manager.go:515:7: duplicate case notify.DefaultSound (constant "" of type string) in expression switch`. `notify.DefaultSound` — константа своя на каждую платформу: `Purr` на darwin, `message-new-instant` на linux и пустая строка в `sender_other.go`, где отправителя нет вовсе. В `setNotifications` она стояла кейсом того же switch, что и `case ""` (тишина), — на платформе без отправителя два кейса схлопываются в один. Теперь это `if cfg.Sound != notify.DefaultSound`, дефолт проверяется первым: где значения совпали, выигрывает отсутствующий ключ, который переживёт переезд конфига на платформу со звуком, а не прибитый `off`. Сломал 6299d65 (`feat(settings): add notification toggles in general tab`), он попадает только в v0.20.0 — в v0.19.0 сборка под Windows была цела, поэтому записи в CHANGELOG нет.

**Дыра в покрытии.** CI собирает и гоняет только ubuntu и macos, кросс-компиляции под Windows нет нигде, кроме релизного goreleaser. Поэтому ошибка дожила до тега. Одна строка `GOOS=windows go build ./...` в джобе lint закрыла бы класс целиком — не делал, это за пределами задачи.

**Флейк на macOS.** `TestChildTabClosePreservesOutcomeAndCannotResurrect/running=true`, `session_child_close_test.go:188` — `followup.Status.Terminal()` ложно. Отменённый follow-up job доматывается на своей горутине и пишет `meta.json` чуть позже, чем закрытое View исчезает из реестра, а тест читал файл один раз сразу после `pumpUntil(registry.Len() == 1)`. Чтение обёрнуто в `pumpUntil` — он крутит цикл на горутине теста; `require.Eventually` здесь брать нельзя, её условие исполняется на отдельной горутине (та же ловушка, что чинили в `lifecycle_ownership_test.go`).

**Проверка.** `GOOS=windows go build ./...` — чисто; `go test -race ./cmd/...` — ok; чинёный тест отдельно `-count=5 -race` — ok; `golangci-lint run` и `fmt --diff` по `internal/harnesssettings` и `cmd` — 0 issues. Первая версия правки была tagless switch, staticcheck поймал её QF1002 — форма именно поэтому `if`, а не `switch`.

**Итог.** Коммит 37c31e6, fast-forward в main (на main включён `required_linear_history`), запушен. Тег v0.20.0 переставлен на 37c31e6.

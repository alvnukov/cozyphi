---
id: ci-windows-cross-build
title: Закрыть дыру в CI — кросс-компиляция под Windows
status: done
priority: medium
tags:
    - ci
    - release
branch: chore/ci-windows-cross-build
worktree_path: .worktrees/ci-windows-cross-build
created_at: "2026-09-06T00:00:00Z"
updated_at: "2026-09-06T00:00:00Z"
---

## Body

Хвост [[fix-windows-build]]: там сборка под Windows развалилась из-за `duplicate case notify.DefaultSound`, и поймал это только goreleaser — уже на теге v0.20.0, ценой двух перезапусков релиза. Причина не в самой ошибке, а в покрытии: матрица тестов — ubuntu и macos, windows-леги в ней нет намеренно (OS-специфичная семантика тестов держала её красной), и больше `GOOS=windows` не трогает никто до релиза.

**Что сделано.** В Makefile таргет `build-windows`: `GOOS=windows CGO_ENABLED=0 go build ./...`. `go build ./...` по множеству пакетов результат отбрасывает, поэтому бинарника после себя не оставляет — это чистая проверка компиляции, не сборка. В джобе `lint` шаг `Cross-compile for Windows` после `make lint`; отдельной джобы не заводил, тулчейн там уже поднят, а несколько секунд компиляции на фоне линта незаметны. Шаги CI в этом репозитории зовут make-таргеты — форма выдержана.

Проверка тестами не подменяется: леги под Windows как не было, так и нет, и вопрос «а проходят ли там тесты» эта задача не решает. Ловится ровно класс «не компилируется».

**Проверка.** `make build-windows` на чистом дереве — зелено, бинарника не осталось. Гард проверен инверсией: временный `internal/notify/zz_probe_windows.go` с `//go:build windows` и неопределённым символом — таргет упал с exit 2 и `undefined: undefinedSymbol`; проба удалена. YAML разобран, шаг стоит в `lint` шестым.

**Итог.** Коммит ab41e77, fast-forward в main.

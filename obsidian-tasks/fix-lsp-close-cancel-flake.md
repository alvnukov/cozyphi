---
id: fix-lsp-close-cancel-flake
title: Flaky TestCloseCancelsPendingAndClosesDocuments under the coverage job
status: done
tags:
    - fix
    - ci
    - lsp
created_at: "2026-09-01T20:10:00Z"
updated_at: "2026-09-02T19:01:40.128556Z"
---

## Body

**Проблема.** В CI-ране 33546675429 (job coverage, ubuntu) упал `internal/lsp/lifecycle_test.go` `TestCloseCancelsPendingAndClosesDocuments` (~5с): `require.Error(t, <-qErr)` получил nil — одна из двух in-flight definition-запросов успела завершиться успешно. Гонка теста: файл-релиз фейк-сервера (`ready+".go"`) пишется сразу после старта горутины `mgr.Close`, и под инструментированной (coverage) сборкой фейк может ответить на запрос раньше, чем Close дойдёт до отмены. На обычных job (ubuntu/macos test) не воспроизводится; в локальном `make test` тоже.

**Что сделать.** Убрать гонку: релизить фейк только после того, как Close гарантированно перевёл менеджер в закрывающееся состояние (например, дождаться маркера отмены/generation-смены или `$/cancelRequest` в history), либо ослабить ассерты — оба запроса должны завершиться (ошибкой или отменой), а строгая проверка остаётся на `$/cancelRequest`/`didClose`/`shutdown` в history.

**Критерий.** Тест стабильно зелёный под `go test -cover ./internal/lsp/ -count=20` и в CI coverage-job; смысл теста (Close отменяет pending и закрывает документы) сохранён.

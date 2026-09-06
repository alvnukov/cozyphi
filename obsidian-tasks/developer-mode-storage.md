---
id: developer-mode-storage
title: 13 — Показать состояние хранилищ без чтения содержимого
status: done
priority: medium
model_level: medium
task_type: feature
parent_id: developer-mode-readonly
tags:
    - developer-mode
acceptance_criteria:
    - 'storage catalog/snapshot/explain покрывает sessions/memory/tasks/usage: scope, безопасные paths/sources, available/loaded и известные счётчики.'
    - Различаются repo/worktree/session/shared-corpus identity без неверного сведения разных workspaces к одному scope.
    - Чтение не сканирует историю, не запускает memory retrieval/index rebuild, task recovery или открытие/создание отсутствующего store.
    - Нет содержимого записей, названий/тел задач, memory text, session messages или raw errors; paths очищены.
    - Нет данных — unset/unavailable с причиной, а не нулевой счётчик; partial results не ломают другие категории.
verification_plan:
    - Fixtures отсутствующих/загруженных/ошибочных stores и shared memory разных worktrees.
    - Spies scan/retrieve/rebuild/open/create/write не вызываются collector; известная stale статистика отмечена.
    - Sentinel в contents/paths/errors отсутствуют в output/audit; доступные соседние категории сохраняются.
created_at: "2026-09-06T09:09:20.781113Z"
updated_at: "2026-09-06T21:47:47.781091Z"
---

## Body

**Сделано.** Категория `storage` отвечает, где лежит состояние сессии и сколько его: транскрипт, memory-корпус, реестр задач репозитория и общая usage-история — 15 ключей, по одному хранилищу за раз (`internal/diag/storage.go` + `storage_sessions.go`, `storage_memory.go`, `storage_tasks.go`, `storage_usage.go`). Каждое хранилище различает состояния, которые иначе читаются одинаково как «ничего нет»: разговор, который кончится вместе с процессом / файл, до которого ещё не дошёл ни один ход / уже записанный; корпус, который не открылся / открылся без индекса / индексирован по пустой директории; репозиторий без реестра / упавшая discovery / процесс, который не искал; история, которая не распарсилась / чистая установка.

**Локаторы.** Только якоря, которые фиксирует layout, плюс placeholders: `~/.cozyphi/session/<workspace>/<session>.jsonl`, `~/.claude/projects/<corpus>/memory`, `<repo>/obsidian-tasks`, `~/.cozyphi/usage.json`. Сегмент, который кодирует чужой путь, не выговаривается никогда — разделителей в нём уже нет, и home-collapse реестра внутрь него не достаёт. Каждый locator отдельно называет identity (`session`/`workspace`/`repository`/`user`), поэтому сессия в linked worktree прямо слышит, что пишет память основного checkout.

**Границы.** Наружу уходят только состояния, счётчики и identity. Нет сообщений, имён/описаний/тел памяти, id/заголовков/тел задач, ключей истории и текстов ошибок. Чтение ничего не открывает и не создаёт, не пересобирает индекс, не достаёт память, не листает session-директорию, не читает заметку и не сканирует историю: счётчик, которого нет в памяти, отвечается как неизвестный, а не как ноль (`tasks.count`, `memory.count`/`memory.kinds` без индекса, `sessions.entries` до первого flush).

**Владельцы.** Проекции живут у владельцев: `internal/session/observe.go`, `internal/memory/observe.go`, `internal/tasks/observe.go`, `internal/usage/observe.go`, `internal/project/observe.go` (якоря из layout, без stat и mkdir), `internal/agent/storage_facts.go`. Провода — `internal/tui/controller/{runtime,controller,diagnostics}.go` и `cmd/run.go`.

**Побочно найдено и починено.** `claudeMemoryRoot` вне Git возвращал стартовую директорию, из-за чего «корпус привязан к checkout» было всегда истинным — теперь возвращает пусто, оба вызывающих и так падали на project root. И один и тот же корпус приходил в двух написаниях (Git резолвит симлинки, рабочая директория — нет), из-за чего checkout отчитывался как worktree самого себя — `Discover` решает это один раз через `os.SameFile`.

**Тесты.** `internal/diag/storage_test.go` (18 тестов, включая закрытый allowlist из 8 написаний locator, просканированный по всем трём слоям каждого поля); `observe_test.go` в пяти пакетах-владельцах — в каждом отдельно проверено, что наблюдение ничего не меняет (файл, написанный за спиной memory-стора, не подхватывается; транскрипт байт в байт; заметка не читается; `usage.json` и `Seen` не трогаются; `StoreAnchors` ничего не создаёт); `internal/tui/controller/developer_mode_storage_test.go` (7 тестов, включая идемпотентность по слоям и разные транскрипты у двух сессий одного workspace); `cmd/run_developer_storage_test.go` (9 тестов на headless-провод: локаторы, уже записанный транскрипт, отсутствующий реестр, счёт корпуса без имён, сломанная история против свежей, каталог из 15 ключей, отсутствие credential и абсолютных путей).

**Gates.** Только по изменённым пакетам: `gofmt -l` чисто; один `golangci-lint run` по девяти пакетам — 0 issues (заодно убраны 2 golines-нарушения, оставшихся от тикета 12, и unparam в `storage_memory.go`); `go test` по девяти пакетам — всё зелено.

**Коммиты.** `17d58b8` + merge `--no-ff` в main. Не пушилось.

## Acceptance Criteria

- storage catalog/snapshot/explain покрывает sessions/memory/tasks/usage: scope, безопасные paths/sources, available/loaded и известные счётчики.
- Различаются repo/worktree/session/shared-corpus identity без неверного сведения разных workspaces к одному scope.
- Чтение не сканирует историю, не запускает memory retrieval/index rebuild, task recovery или открытие/создание отсутствующего store.
- Нет содержимого записей, названий/тел задач, memory text, session messages или raw errors; paths очищены.
- Нет данных — unset/unavailable с причиной, а не нулевой счётчик; partial results не ломают другие категории.

## Verification Plan

1. Fixtures отсутствующих/загруженных/ошибочных stores и shared memory разных worktrees.
2. Spies scan/retrieve/rebuild/open/create/write не вызываются collector; известная stale статистика отмечена.
3. Sentinel в contents/paths/errors отсутствуют в output/audit; доступные соседние категории сохраняются.

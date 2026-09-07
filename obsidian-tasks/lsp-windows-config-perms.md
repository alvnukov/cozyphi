---
id: lsp-windows-config-perms
title: LSP-конфиг на Windows — unix-гард прав больше не отвергает каждый файл
status: done
priority: high
model_level: medium
task_type: bug
tags:
    - lsp
    - windows
branch: fix/lsp-windows
worktree_path: .worktrees/cozy-tools-require
acceptance_criteria:
    - Гард group/world-writable вынесен за OS-сплит; на Windows это no-op (Windows не несёт unix-битов прав), конфиг грузится.
    - На unix проверка не регрессирует — group- и world-writable файлы по-прежнему отвергаются с ошибкой "writable".
    - CHANGELOG Unreleased, текст на английском.
verification_plan:
    - go build ./internal/lsp/...; GOOS=windows go build ./internal/lsp/...; go test ./internal/lsp/...
created_at: "2026-09-07T16:05:00Z"
updated_at: "2026-09-07T16:05:00Z"
---

## Body

**Откуда.** В `LoadConfig` (`config.go`) стоял `if fst.Mode().Perm()&0o022 != 0 { ... "must not be group- or world-writable" }`. На unix это осмысленный гард против эскалации. На Windows любой файл рапортует синтетический режим `0666`, поэтому проверка срабатывала на КАЖДОМ конфиге — ни один `lsp.json` там не мог загрузиться, и весь LSP оставался без конфигурации.

**Что сделано.** По идиоме пакета (`exec_unix.go`/`exec_windows.go`, `config_unix.go`/`config_windows.go`) проверка вынесена за build-tag сплит: `configWorldOrGroupWritable(fi os.FileInfo) bool`. На unix (`config_unix.go`) он делает настоящую `Perm()&0o022 != 0`; на Windows (`config_windows.go`) всегда возвращает false — режимы там ACL-овые, unix-биты не несут смысла. Формулировка ошибки и стиль комментариев выдержаны по файлу. Два подтеста `group-writable`/`world-writable` в `config_test.go` скипаются на Windows (`runtime.GOOS`), потому что там нельзя создать реальный 0660/0666 файл, а гард намеренно no-op.

**Проверка.** `go build ./internal/lsp/...` и `GOOS=windows CGO_ENABLED=0 go build ./internal/lsp/...` — оба ок (windows-обёртка компилируется, unused-параметр `fi` допустим, как у соседнего `configOwnedByCurrentUser`). `go test ./internal/lsp/...` — зелёный на macOS, unix-подтесты writable по-прежнему ловят ошибку. Ветка `fix/lsp-windows` от `main`, worktree `.worktrees/cozy-tools-require`. Приземление и проверку на реальном Windows делает супервизор.

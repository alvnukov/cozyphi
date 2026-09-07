---
id: lsp-windows-file-uri
title: LSP на Windows — правильная сборка file:// URI для gopls
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
    - Windows-путь C:\Users\zx\main.go кодируется в file:///C:/Users/zx/main.go (тройной слэш, прямые слэши, литеральное двоеточие диска), декодируется обратно; POSIX не регрессирует.
    - Декодер принимает и процент-кодированное двоеточие диска (file:///C%3A/...) и нормализует его в C:\Users\zx\main.go.
    - Табличные round-trip тесты покрывают POSIX и Windows детерминированно и проходят на macOS.
    - CHANGELOG Unreleased, текст на английском.
verification_plan:
    - go build ./internal/lsp/...; go test ./internal/lsp/...
created_at: "2026-09-07T16:00:00Z"
updated_at: "2026-09-07T16:00:00Z"
---

## Body

**Откуда.** На Windows `uriFromPath` заворачивал очищенный абсолютный путь в `file://` + `fileURIEscape`, а тот процент-кодировал двоеточие диска и бэкслэши: `C:\Users\zx\main.go` → `file://C%3A%5CUsers%5Czx%5Cmain.go`. Такой URI `url.Parse` (и сам gopls) отвергает — на Windows падал каждый LSP-запрос, ни go-to-definition, ни hover, ни диагностика не отвечали. Декодер тоже был сломан: `url.Parse("file:///C:/Users/zx/main.go").Path` == `/C:/Users/zx/main.go`, и `filepath.IsAbs` этого на Windows не признаёт, `filepath.Clean` не даёт `C:\...` — ведущий слэш перед буквой диска нужно снять первым.

**Что сделано.** OS-специфичная нормализация вынесена в чистые функции с явным флагом `windows`, чтобы тесты гоняли обе ветки детерминированно с POSIX-хоста. Кодирование: `toURIPath(path, windows)` (`position.go`) переводит бэкслэши в слэши и гарантирует ведущий слэш, так что `C:\...` → `/C:/...`; `fileURIEscape` теперь оставляет `:` литеральным (в path URI это легально по RFC 3986), спецбайты вроде пробела по-прежнему `%20`. Декодирование: `osPathFromURIPath(path, windows)` (`uri.go`) снимает ведущий слэш перед буквой диска и меняет слэши на бэкслэши; `pathFromURI` вызывает его перед `filepath.IsAbs`/`Clean`. Процент-кодированное двоеточие приходит уже декодированным в `u.Path`, так что обе формы сходятся. POSIX-ветка — тождество, поведение не меняется.

**Тонкости.** Слэш-конверсия для Windows делается руками (`strings.ReplaceAll`), а не через `filepath.FromSlash`, потому что тот host-специфичен и на macOS был бы no-op — иначе Windows-кейс на CI не проверить. Реальный флаг платформы даёт `runtime.GOOS == "windows"` в тонких обёртках. `FuzzPathFromURI` остаётся зелёным: на не-Windows декодер — тождество.

**Проверка.** `go build ./internal/lsp/...` — ок. Табличный `TestFileURIRoundTrip` (POSIX plain/space, Windows plain/space) и `TestFileURIDecodesPercentEncodedDriveColon` — зелёные на macOS. Ветка `fix/lsp-windows` от `main`, worktree `.worktrees/cozy-tools-require`. Приземление и проверку на реальном Windows делает супервизор.

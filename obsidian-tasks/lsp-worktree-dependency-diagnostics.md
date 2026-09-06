---
id: lsp-worktree-dependency-diagnostics
title: Fix stale dependency diagnostics in worktree modules
status: done
priority: low
model_level: medium
task_type: bug
tags:
    - lsp
    - harness
branch: bug/lsp-worktree-dependency-diagnostics
worktree_path: .worktrees/lsp-worktree-dependency-diagnostics
acceptance_criteria:
    - Worktree diagnostics reflect edited nested-module dependencies or disclose stale provenance.
verification_plan:
    - Reproduce dependency declaration edits in nested xui module and compare diagnostics with go test.
created_at: "2026-09-05T23:36:19.4203Z"
updated_at: "2026-09-06T10:12:22.318462Z"
---

## Body

During safe-paste-limit in .worktrees/safe-paste-limit, scoped go test ./input (xui) and root chat/composer/editor all passed, but harness lsp diagnostics labeled fresh report undefined input.MaxPasteBytes and xui.PasteRejectedEvent. Both declarations exist in edited xui/input/parser.go and event.go/alias.go. Explicit diagnostics of event.go/alias.go report none; root chat still reports undefined constant. Investigate worktree/nested-module dependency synchronization and provenance.

**Note (2026-09-06).** Повторное наблюдение в .worktrees/fix-tui-agent-session-deletion: parent lsp diagnostics помечал fresh undefined Editor.closing/closeConfirm, хотя поля уже присутствовали и go test -race пакета проходил. После синхронизации editor.go диагностика сначала оставалась cached. Аналогично lifecycle.go сообщает undefined StatusHistory.BeginClose до явной синхронизации controller/status.go; go test -race всех пяти затронутых пакетов проходит. Похоже на уже зарегистрированную проблему синхронизации зависимостей, теперь и внутри одного root module, не только nested xui. Ошибки LSP не использовать как доказательство дефекта закрытия вкладок.

**Note (2026-09-06).** Подтверждено в .worktrees/multisession-title-entry (base c7c5f5ae): go test ./internal/components/app ./internal/tui/sessions прошёл (1.121s/46.052s), но lsp diagnostics internal/components/app/app.go:33:11 возвращает fresh undefined: terminalTitle; объявление находится в новом internal/components/app/title.go, диагностика самого title.go ранее none (fresh). Повтор app.go возвращает ту же ошибку cached. Одновременно sessions/view.go:355:11 сообщает fresh e.footer.SetSessionTitle undefined, хотя footer/footer.go содержит метод и его диагностика none (fresh). Это уже root-module/new-file synchronization, не только nested modules. Работа над session titles приостановлена по правилу пользователя; требуется отдельное исправление харнесса до возврата.

**Started (2026-09-06).** Отдельное исправление блокирующего бага по одобренному repair-lsp; незавершённый worktree multisession-title-entry не трогать.

**Done (2026-09-06).** Исправление доставлено в main merge b5e7bb6, code a754f28 (интеграция актуального main в worktree 7379f68; конфликт только CHANGELOG, обе записи сохранены). Query-driven bounded scan Go/module files сообщает gopls additions/changes/deletions; обновляет открытые dependency overlays; инвалидирует derived diagnostics, продвигает target versions и помечает concurrent source sync как unconfirmed. Исправлена потеря wakeup между проверкой диагностики и подпиской. Red real-gopls repro: undefined dep.New status=fresh; green same-module+nested replace, create/delete. Проверки: весь internal/lsp -race 34.720s; реальные TestRealGopls 4.422s; scoped fmt; ЕДИНСТВЕННЫЙ golangci-lint run ./internal/lsp — 0 issues. После финального безопасного чтения doc.notified под lock повторены targeted race и real regressions — green. Ограничения scan описаны в doc/lsp.md. Текущий процесс cozyphi использует старый клиент: для применения нужна новая сборка и перезапуск cozyphi, не только gopls. Feature multisession-title-entry остаётся незакоммиченным в своём worktree.

## Acceptance Criteria

- Worktree diagnostics reflect edited nested-module dependencies or disclose stale provenance.

## Verification Plan

1. Reproduce dependency declaration edits in nested xui module and compare diagnostics with go test.

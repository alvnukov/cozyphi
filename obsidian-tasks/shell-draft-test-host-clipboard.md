---
id: shell-draft-test-host-clipboard
title: Keep shell draft-retention test independent of host clipboard
status: done
priority: medium
model_level: low
task_type: bug
parent_id: multisession-interactive-children
tags:
    - test
    - multisession
acceptance_criteria:
    - Draft-retention test does not read the real OS clipboard.
    - The existing background Ask/focus/draft assertions still pass under race.
verification_plan:
    - go test -race ./internal/tui/editor -run '^TestShellRetainsDraftsAndDrainsBackgroundAsk$' -count=10
    - Include internal/tui/editor in the child-session integration race run.
created_at: "2026-09-05T20:04:11.695548Z"
updated_at: "2026-09-05T20:27:39.261002Z"
---

## Body

W17 child integration run exposed TestShellRetainsDraftsAndDrainsBackgroundAsk depending on the real host clipboard: its synthetic PasteEvent imported an image/png attachment instead of draftBeta. The draft-isolation test is not a clipboard test.

**Done (2026-09-05).** Закрыто merge'ем 7024b9a (задача multisession-review-fixes): в composer/View добавлен seam SetClipboardReader, единственное чтение системного буфера в ComposerPane идёт через поле readClipboard (pane.go:237), а TestShellRetainsDraftsAndDrainsBackgroundAsk на main подменяет его заглушкой без картинки (shell_test.go:27). Синтетический PasteEvent оставлен: он больше не касается хоста. Проверка на main: go test -race ./internal/tui/editor -run '^TestShellRetainsDraftsAndDrainsBackgroundAsk$' -count=10 зелёный. Отдельного кода в этой задаче нет. Незакоммиченная правка в .worktrees/multisession-interactive-children/internal/tui/editor/shell_test.go (замена paste на rune-события) теперь избыточна и при merge той ветки в main даст конфликт в этом тесте: брать версию main.

## Acceptance Criteria

- Draft-retention test does not read the real OS clipboard.
- The existing background Ask/focus/draft assertions still pass under race.

## Verification Plan

1. go test -race ./internal/tui/editor -run '^TestShellRetainsDraftsAndDrainsBackgroundAsk$' -count=10
2. Include internal/tui/editor in the child-session integration race run.

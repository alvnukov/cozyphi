---
id: theme-switch-background
title: 'Fix /theme switching: overlay panes + app background'
status: in_progress
priority: high
task_type: bug
tags:
    - tui
    - theme
branch: bug/theme-switch-background
worktree_path: .worktrees/theme-switch-background
created_at: "2026-09-10T11:40:05.574715Z"
updated_at: "2026-09-12T15:40:41.339479Z"
---

## Body

**What**: `/theme` in cozyphi does not restyle the UI. Two layers found:

1. Overlay panes (ctxpane, helppane, watchpane, usagepane, statuspane, agentlist) are built on the startup theme and were never called from `View.ApplyTheme` , fix already written in the working tree (SetTheme methods + wiring + regression test `TestThemeSwitchRestylesTheStatusDashboard` in internal/tui/sessions/observe_test.go).
2. **Background never switches**: no theme defines an app background , `Theme` has no root `Background` style, all palettes are Fg-only (legacy `legacyChrome` even forces `Bg: DefaultColor` on panels, theme.go:255-256). Screen fills use `theme.Foreground` (no Bg) , the terminal paints the background. Fix: add `Background xui.Style` to `components.Theme`, set per palette (Terminal stays DefaultColor by design), fill root surfaces with it (statuspane buffer fill pane.go:338, settings/pane.go:1195, planedit/pane.go:2320, app root from Editor.Draw editor.go:205 / View.Draw view.go:1206).

**Where**: branch `fix/theme-switch-background` in the main checkout (user directive: no code work directly on main). Worktree `.worktrees/light-theme-redesign` (feature/light-theme-redesign) is the second stop for vs-light/opencode-light consistency.

**Done so far**: uncommitted changes in main checkout = five SetTheme methods + + ApplyTheme wiring + test. Not built/tested yet (bash timed out earlier).

**Started (2026-09-10).** Работа перенесена с main на ветку fix/theme-switch-background (правило пользователя: код , только в ветке под задачей). Незакоммиченный фикс панелей уже на ветке.

**Note (2026-09-10).** 2026-09-10: основной фикс закоммичен в ветке fix/theme-switch-background (main-checkout): fix(tui): SetTheme для 6 оверлей-панелей + проводка ApplyTheme + Theme.Background (opencode 0x0a0a0a, opencode-light 0xffffff, Dark 0x1e1e1e, Darcula 0x2b2b2b, Pink 0x23151b, Terminal default) + заливка корня View.Draw, statuspane, settings/planedit; go build + 10 пакетов тестов зелёные. Осталось: порт в воркдрей light-theme-redesign (vs-light/opencode-light согласованность).

**Note (2026-09-12).** 2026-09-10: проверки на ветке fix/theme-switch-background зелёные: go build ./..., go test (components + sessions, agentlist, ctxpane, planedit, settings, statuspane, usagepane, watchpane), make fmt-check, всё exit 0. Порт в light-theme-redesign отменён как невозможный: ветки и воркдрея не существует, незакоммиченная работа VS Light от 2026-09-05 потеряна вместе с воркдреем (проверены branch -a, stash, reflog, dangling-коммиты, mdfind; соседнее попадание mdfind это чужой docs-репозиторий). opencode-light согласован самим фиксом f94a13c (Background #ffffff). Задача закрыта: пользователь собирает бинарь из основного чекаута (ветка fix/theme-switch-background) и проверяет /theme локально.

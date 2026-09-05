---
id: status-dashboard
title: Add /status dashboard with Status, Config, Usage and Stats
status: done
priority: high
model_level: high
task_type: feature
tags:
    - tui
branch: feature/status-dashboard
worktree_path: .worktrees/status-dashboard
acceptance_criteria:
    - /status opens Status, Config, Usage and Stats tabs.
    - Config searches and persists existing settings.
    - Usage and historical statistics use real data with explicit unavailable values.
    - Keyboard navigation, resizing and focused tests are covered.
verification_plan:
    - Inspect command routing, settings persistence and usage records.
    - Test dashboard navigation, settings persistence and statistics boundaries.
    - Run formatting, lint and tests; review and commit owned files.
created_at: "2026-09-05T11:31:21.981166Z"
updated_at: "2026-09-05T12:46:18.179594Z"
---

## Body

Implement the approved four-tab /status interface inspired by Claude Code. Reuse cozyphi settings, controller and usage persistence; do not copy Claude-specific settings or invent quotas. Isolate code in a task worktree. Add tests, documentation and changelog.

**Note (2026-09-05).** Уточнение пользователя: /status открывается на вкладке, на которой dashboard чаще всего закрывали. Учитывать именно вкладку в момент закрытия, а не просмотры или последнюю выбранную вкладку. При отсутствии истории открывать Status; при равных счётчиках использовать стабильный порядок вкладок. Счётчики сохранять между запусками как UI preference.

**Note (2026-09-05).** Обновление требования: без истории и при равных счётчиках приоритет Usage (заменяет прежний default Status). Добавлены UIState.StatusCloses, PreferredStatusTab/RecordStatusClose и тесты persistence/ties/overflow; go test ./internal/project прошёл. Параллельно реализуются read-only история и TUI в task-worktree.

**Note (2026-09-05).** Added Codex /usage support in `.worktrees/status-dashboard` using the existing provider quota seam: OpenAI OAuth credentials are refreshed, Codex usage and reset-credit GET endpoints are queried without performing reset mutations, and the usage pane renders percent limits, token totals, reset timing, and reset-consumption limitation. Verification: `go test ./internal/provider ./internal/tui/usagepane ./...` passed in the worktree.

**Note (2026-09-05).** Implementation is wired in .worktrees/status-dashboard: four tabs, embedded settings, provider quota isolation, read-only bounded history with cancellation/generation guards, persistent actual-close counts. Usage wins when tied for maximum and defaults with no history; other equal maxima use stable Status/Config/Stats order. Independent Standards review found no hard violations; Spec review found lost Config paste input, fixed with regression coverage. Model/LSP source reporting restored with secret-exclusion tests. Final formatting/lint/full/race checks running. Foreign Codex quota commit 3a62b4a appeared on branch during work; preserve it and main's unrelated edits.

**Note (2026-09-05).** User confirmed gates must cover changes only. Stopped whole-repo sweep; removed formatter-only changes to unrelated provider/usagepane files. Final gate: format-check staged Go files, golangci-lint --new-from-rev=HEAD on seven touched packages, tests for those packages; affected package race run passed, final Stats-summary race rerun queued. Added explicit recorded active UTC days, longest streak and favorite known model to Stats. Re-review of Config paste/source reporting/preference behavior found no remaining actionable defects. go.sum remains unstaged and excluded.

**Done (2026-09-05).** Completed in feature/status-dashboard at 6df74d7 (worktree .worktrees/status-dashboard retained as requested). Four-tab dashboard, embedded searchable editable settings, safe runtime sources, real provider/session usage, read-only cancellable historical Stats with periods/heatmap/models/activity summary, persisted actual-close preference (Usage default and max-count tie priority). Documentation and changelog included. Standards/Spec reviews completed; paste/source findings fixed and re-reviewed. Final changed-file format-check and scoped lint passed with 0 issues; go test -mod=readonly passed for cmd/project/session/commands/controller/editor/statuspane; final -race passed for session/controller/editor/statuspane, earlier race also passed project/commands. Worktree clean; no go.sum or unrelated provider formatting included. No merge or push performed; main code and other task files untouched.

**Reopened (2026-09-05).** Преждевременное закрытие исправлено: код проверен и закоммичен, но ещё не слит в main. Завершение требует merge feature/status-dashboard, фиксации ledger и удаления чистого task-worktree/ветки. Push не требуется.

**Started (2026-09-05).** Продолжаю финальную интеграцию: merge проверенной task-ветки в main и cleanup; код повторно не меняется.

**Done (2026-09-05).** Поставка действительно завершена: feature/status-dashboard слита в main merge-коммитом 99f3065 (реализация 6df74d7 является предком main). Merge прошёл без конфликтов; ранее пройденные scoped-гейты повторно не запускались. Чистый worktree .worktrees/status-dashboard удалён, слитая ветка feature/status-dashboard удалена. SHA-256 go.sum и чужих задач agent-plan-effort-only/light-theme-redesign до и после merge совпадают. Push не выполнялся. Эта запись заменяет прежнее ошибочное утверждение, что branch-only поставка завершает задачу.

## Acceptance Criteria

- /status opens Status, Config, Usage and Stats tabs.
- Config searches and persists existing settings.
- Usage and historical statistics use real data with explicit unavailable values.
- Keyboard navigation, resizing and focused tests are covered.

## Verification Plan

1. Inspect command routing, settings persistence and usage records.
2. Test dashboard navigation, settings persistence and statistics boundaries.
3. Run formatting, lint and tests; review and commit owned files.

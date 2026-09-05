---
id: status-dashboard
title: Add /status dashboard with Status, Config, Usage and Stats
status: in_progress
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
updated_at: "2026-09-05T12:12:06.994104Z"
---

## Body

Implement the approved four-tab /status interface inspired by Claude Code. Reuse cozyphi settings, controller and usage persistence; do not copy Claude-specific settings or invent quotas. Isolate code in a task worktree. Add tests, documentation and changelog.

**Note (2026-09-05).** Уточнение пользователя: /status открывается на вкладке, на которой dashboard чаще всего закрывали. Учитывать именно вкладку в момент закрытия, а не просмотры или последнюю выбранную вкладку. При отсутствии истории открывать Status; при равных счётчиках использовать стабильный порядок вкладок. Счётчики сохранять между запусками как UI preference.

**Note (2026-09-05).** Обновление требования: без истории и при равных счётчиках приоритет Usage (заменяет прежний default Status). Добавлены UIState.StatusCloses, PreferredStatusTab/RecordStatusClose и тесты persistence/ties/overflow; go test ./internal/project прошёл. Параллельно реализуются read-only история и TUI в task-worktree.

**Note (2026-09-05).** Added Codex /usage support in `.worktrees/status-dashboard` using the existing provider quota seam: OpenAI OAuth credentials are refreshed, Codex usage and reset-credit GET endpoints are queried without performing reset mutations, and the usage pane renders percent limits, token totals, reset timing, and reset-consumption limitation. Verification: `go test ./internal/provider ./internal/tui/usagepane ./...` passed in the worktree.

## Acceptance Criteria

- /status opens Status, Config, Usage and Stats tabs.
- Config searches and persists existing settings.
- Usage and historical statistics use real data with explicit unavailable values.
- Keyboard navigation, resizing and focused tests are covered.

## Verification Plan

1. Inspect command routing, settings persistence and usage records.
2. Test dashboard navigation, settings persistence and statistics boundaries.
3. Run formatting, lint and tests; review and commit owned files.

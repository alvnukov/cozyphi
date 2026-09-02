---
id: desktop-notifications
title: Desktop push notification when the model stops or asks the user
status: done
priority: high
task_type: feature
tags:
    - tui
    - ux
    - notifications
    - phase1
acceptance_criteria:
    - A finished or stopped turn fires one OS notification; permission, continue and question asks fire one with context.
    - notifications.mode off|always|unfocused honored; absent key means always; invalid value fails config load.
    - darwin uses osascript display notification, linux uses notify-send; title/body passed as argv; send failure logs once and suppresses retries.
    - make fmt-check lint test green in the task worktree; CHANGELOG [Unreleased] line added; conventional commit on the feature branch.
verification_plan:
    - go test ./internal/notify/ ./internal/project/ ./internal/tui/editor/ ./cmd/
    - make fmt-check lint test in the task worktree
created_at: "2026-08-30T21:45:07.947239Z"
updated_at: "2026-08-30T22:21:10.206617Z"
---

## Body

When the agent finishes (or stops) a turn, or asks the user for something (permission gate, continue-ask, question tool), send a desktop push notification so the user notices without watching the terminal.

Design: new internal/notify package behind a small surface (SetFocused / TurnEnded / NeedsAttention); darwin osascript and linux notify-send senders, async fire-and-forget; Editor tracks terminal focus via xui.FocusEvent and triggers on RunEndedMsg + the three Ask messages; notifications.mode knob in config.yaml (off|always|unfocused, default always).

## Acceptance Criteria

- A finished or stopped turn fires one OS notification; permission, continue and question asks fire one with context.
- notifications.mode off|always|unfocused honored; absent key means always; invalid value fails config load.
- darwin uses osascript display notification, linux uses notify-send; title/body passed as argv; send failure logs once and suppresses retries.
- make fmt-check lint test green in the task worktree; CHANGELOG [Unreleased] line added; conventional commit on the feature branch.

## Verification Plan

1. go test ./internal/notify/ ./internal/project/ ./internal/tui/editor/ ./cmd/
2. make fmt-check lint test in the task worktree

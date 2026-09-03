---
id: resume-same-model
title: Resume a session with the same model
status: done
priority: high
task_type: bug
tags:
    - session
    - tui
created_at: "2026-08-25T02:00:00.000000Z"
updated_at: "2026-08-25T02:30:00.000000Z"
---

## Body

Resuming a persisted session (TUI `/resume`, `cozyphi --resume`, `cozyphi run
--session/--continue-last`) falls back to the configured default model instead
of the model the session was using. Persist the session's model and restore it
when the session is reopened so the same model continues the conversation.

source_repo_path: /Users/zol/src/cozyphi

resolved_by: b90e78c feat(session): resume with the model the session used

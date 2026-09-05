---
id: codex-usage-reset-button
title: Compact Codex usage and add confirmed reset button
status: done
priority: high
model_level: high
task_type: feature
tags:
    - provider
    - usage
branch: feature/codex-usage-reset-button
worktree_path: .worktrees/codex-usage-reset-button
acceptance_criteria:
    - Compact /usage without daily/lifetime profile token rows.
    - User-triggered reset button uses official API with explicit confirmation, no duplicate sends, refresh after success.
    - No live reset executed during development; scoped gates pass; commit merge and clean task isolation.
verification_plan:
    - Inspect pinned official reset source and UI integration.
    - Local fixture tests for reset contract and confirmation/error/concurrency behavior.
    - Changed-file formatting, diff-scoped lint and tests of touched packages only.
    - Commit source, merge main, commit ledger, remove task branch/worktree; no push.
created_at: "2026-09-05T13:28:02.865459Z"
updated_at: "2026-09-05T13:57:35.568881Z"
---

## Body

Replace the verbose Codex usage report with a compact limits/reset summary and implement a user-operated reset button with confirmation. User authorizes implementing mutations behind explicit UI confirmation, not executing real resets during development. Consult official Codex source; preserve concurrent dashboard layout work.

**Note (2026-09-05).** Implementation committed as 41429c7. Official source pinned at openai/codex 531f3836a1e38ea61eaaba3dccda6711eb6c0dca; POST contract verified from source, not live reset compatibility. Standards/spec reviews completed; invisible/queued confirmation and automatic-refresh re-enable findings fixed with regression tests. Scoped lint reports 0 issues; race tests pass for provider, controller, usagepane, statuspane and editor. All mutation tests use synthetic credentials/local HTTP servers; no real reset or credit spending. Merged current main into task branch without conflicts to preserve independently landed dashboard corrections; repeating scoped gates on combined code before integration.

**Done (2026-09-05).** Landed in main via 8937eee; implementation 41429c7, integration test adaptation 2749b08. Compact standalone/dashboard usage removes profile-token history; standalone reset requires visible one-credit confirmation with account-bound single-use authorization, no ambiguous retries, and subsequent quota refresh. Both review findings have regression coverage. After incorporating current main/dashboard changes, scoped lint reports 0 issues and all five affected packages pass go test -race; changed-file formatting and diff checks pass. Mutation compatibility is official-source/fixture verified only: no real resets or credit consumption, no push. Unrelated go.sum and parallel task notes preserved.

## Acceptance Criteria

- Compact /usage without daily/lifetime profile token rows.
- User-triggered reset button uses official API with explicit confirmation, no duplicate sends, refresh after success.
- No live reset executed during development; scoped gates pass; commit merge and clean task isolation.

## Verification Plan

1. Inspect pinned official reset source and UI integration.
2. Local fixture tests for reset contract and confirmation/error/concurrency behavior.
3. Changed-file formatting, diff-scoped lint and tests of touched packages only.
4. Commit source, merge main, commit ledger, remove task branch/worktree; no push.

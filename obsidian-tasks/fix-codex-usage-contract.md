---
id: fix-codex-usage-contract
title: Fix Codex usage against the official API contract
status: done
priority: high
model_level: high
task_type: bug
tags:
    - provider
branch: bug/fix-codex-usage-contract
worktree_path: .worktrees/fix-codex-usage-contract
acceptance_criteria:
    - Codex usage uses documented upstream endpoint and schema, with regression coverage.
    - No invented tokens, reset APIs or secret exposure.
verification_plan:
    - Reproduce current failure with upstream contract fixture.
    - Run scoped tests and changed-code lint.
    - Commit owned files and task ledger.
created_at: "2026-09-05T12:50:01.617422Z"
updated_at: "2026-09-05T13:20:00.557046Z"
---

## Body

Fix nonworking Codex /usage introduced by 3a62b4a, now merged through status-dashboard. Establish official contract before implementation; preserve other task work.

**Note (2026-09-05).** Confirmed contract from official openai/codex at 531f3836a1e38ea61eaaba3dccda6711eb6c0dca (app-server documentation, backend client and generated HTTP wire models). Old route reproduced HTTP 404 in upstream-contract regression. Corrected WHAM usage/profile adapter passed a read-only live probe: both endpoints HTTP 200; decoded limits, profile token scopes and reset summary. Probe allowed only exact HTTPS GETs, refused expired credentials, and logged no credentials, bodies, identifiers or account usage values. No refresh/reset/account mutations. Live profile contained 153 token scopes, exposing a pane overflow; adding scrolling before final checks. Source evidence and limited live scope recorded in doc/codex-usage.md. Prior synthetic fixtures were not integration proof.

**Note (2026-09-05).** Fixed actual dashboard integration: usageLines now renders the complete report instead of clipping to 64 rows. Added 153-bucket regression proving final daily bucket, reset summary and session remain reachable. Independent review found no blockers; provider/usagepane/statuspane tests passed. Stopped whole-repo verification on user correction; final gates are changed-file format, diff-scoped lint and race tests of those three packages only. Corrected test-only lint findings (request contexts, status constants, ErrorIs, range-int).

**Done (2026-09-05).** Landed source commit d6337de and merge 8bd622d in main. Correct WHAM usage/profile contract, explicit scoped/missing tokens, remaining percentages and reset timing/credits, complete scrollable report in /usage/dashboard. Live read-only probe verified both GET endpoints HTTP 200; no account mutations. Final changed-file formatting and diff-scoped lint passed (0 issues), as did race tests for provider, usagepane and statuspane. Task worktree removed and branch deleted; unrelated main go.sum and other tasks preserved. No push.

## Acceptance Criteria

- Codex usage uses documented upstream endpoint and schema, with regression coverage.
- No invented tokens, reset APIs or secret exposure.

## Verification Plan

1. Reproduce current failure with upstream contract fixture.
2. Run scoped tests and changed-code lint.
3. Commit owned files and task ledger.

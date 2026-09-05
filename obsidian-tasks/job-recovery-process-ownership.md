---
id: job-recovery-process-ownership
title: Protect live job recovery with process ownership
status: todo
priority: high
model_level: high
task_type: bug
parent_id: multisession-mode
tags:
    - jobs
    - reliability
acceptance_criteria:
    - Opening a second process job manager does not mark another process's live assignments failed.
    - After a true process restart, abandoned assignments have explicit terminal/recovery state and do not resume autonomous work.
    - Cross-process ownership recovery is tested without relying only on one-manager-per-process construction.
verification_plan:
    - Run two Managers against the same root with a controlled live runner; opening the second must preserve the first outcome.
    - Simulate an abandoned owner and verify terminal recovery and no automatic restart.
    - Run scoped job race tests.
created_at: "2026-09-05T15:46:17.335889Z"
updated_at: "2026-09-05T15:46:17.335889Z"
---

## Body

Runtime-split audit found Manager.recoverStale marks every nonterminal job in shared ~/.cozyphi/jobs failed whenever a Manager opens, without process-owner evidence. One process-owned manager fixes duplicate recovery inside the TUI but not concurrent CLI/TUI processes. Evidence: internal/job/manager.go New/recoverStale; internal/project/project.go JobsDir. Address in child-session hardening if necessary for the approved recovery contract; otherwise leave this issue open and disclose the limitation. Do not hide stale jobs with RecoverIgnore.

## Acceptance Criteria

- Opening a second process job manager does not mark another process's live assignments failed.
- After a true process restart, abandoned assignments have explicit terminal/recovery state and do not resume autonomous work.
- Cross-process ownership recovery is tested without relying only on one-manager-per-process construction.

## Verification Plan

1. Run two Managers against the same root with a controlled live runner; opening the second must preserve the first outcome.
2. Simulate an abandoned owner and verify terminal recovery and no automatic restart.
3. Run scoped job race tests.

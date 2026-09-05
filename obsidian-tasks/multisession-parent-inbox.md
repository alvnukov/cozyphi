---
id: multisession-parent-inbox
title: Deliver child outcomes reliably and wake the parent session
status: todo
priority: high
model_level: high
task_type: feature
parent_id: multisession-mode
tags:
    - multisession
    - agents
acceptance_criteria:
    - Completion/error/stop reaches the exact parent as a correlated bounded outcome, without lossy progress/watch queue transport.
    - Active parents receive outcomes at an inference/tool seam; idle parents wake without focus theft; explicit stop suppresses autonomous continuation while preserving pending outcomes.
    - Wait/push deduplication, burst handling, persistence failures and process-owned recovery have tested semantics; headless wait remains supported.
verification_plan:
    - Controlled active/idle/final-boundary and user-stop delivery tests.
    - Burst, duplicate, wait/push, full-backlog and persistence-failure tests.
    - Focused job/agent/controller race checks plus headless regression tests.
created_at: "2026-09-05T15:19:29.517864Z"
updated_at: "2026-09-05T15:19:29.517864Z"
---

## Body

Implement the reliable parent inbox contract from obsidian-tasks/interactive-child-sessions-design.md. Reuse wake scheduling, not the drop-oldest watch queue. Persist assignment outcome and delivery identity, expose undelivered persistence failures, bound in-memory backlog and guard shutdown/recovery ownership. Treat summaries as child output, never a user instruction or permission approval. **Blocked by:** multisession-interactive-children. Coordinate UI attention with multisession-background-attention without conflating UI notices with model inbox delivery. **Effort:** high. Code only in the task worktree.

## Acceptance Criteria

- Completion/error/stop reaches the exact parent as a correlated bounded outcome, without lossy progress/watch queue transport.
- Active parents receive outcomes at an inference/tool seam; idle parents wake without focus theft; explicit stop suppresses autonomous continuation while preserving pending outcomes.
- Wait/push deduplication, burst handling, persistence failures and process-owned recovery have tested semantics; headless wait remains supported.

## Verification Plan

1. Controlled active/idle/final-boundary and user-stop delivery tests.
2. Burst, duplicate, wait/push, full-backlog and persistence-failure tests.
3. Focused job/agent/controller race checks plus headless regression tests.

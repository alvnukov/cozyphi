---
id: tui-drops-enter-while-model-is-running-instead-of-queueing-prompts
title: TUI drops Enter while model is running instead of queueing prompts
status: done
priority: high
task_type: bug
tags:
    - issue
    - feedback
    - tui
    - queue
    - reliability
    - ux
acceptance_criteria:
    - Prompts submitted during an active model run execute FIFO and Engine.Loop runs never overlap.
    - 'Submit never silently drops input: model-busy prompts queue; bash conflicts produce explicit feedback and preserve input.'
    - Cancel and lifecycle operations cannot open a concurrent-run window or silently lose queued prompt data.
    - Queued skill selections are immutable snapshots rather than aliased composer storage.
    - Background streaming redraw work is paced from evidence while direct keyboard redraw remains immediate.
verification_plan:
    - Run focused controller and submit tests covering FIFO, cancel, bash conflict, and skill snapshots.
    - Run focused tests with the Go race detector.
    - Run make fmt-check lint test after focused gates pass.
created_at: "2026-08-24T08:34:10.080498Z"
updated_at: "2026-08-24T09:04:14.242104Z"
---

## Body

Audit and harden the merged prompt queue across composer, submitter, controller, cancellation, lifecycle operations, and UI render pacing. Keep the queue owned by Controller, the owner of sequential Engine.Loop execution. Do not modify the opencode composer/frame implementation.

## Acceptance Criteria

- Prompts submitted during an active model run execute FIFO and Engine.Loop runs never overlap.
- Submit never silently drops input: model-busy prompts queue; bash conflicts produce explicit feedback and preserve input.
- Cancel and lifecycle operations cannot open a concurrent-run window or silently lose queued prompt data.
- Queued skill selections are immutable snapshots rather than aliased composer storage.
- Background streaming redraw work is paced from evidence while direct keyboard redraw remains immediate.

## Verification Plan

1. Run focused controller and submit tests covering FIFO, cancel, bash conflict, and skill snapshots.
2. Run focused tests with the Go race detector.
3. Run make fmt-check lint test after focused gates pass.

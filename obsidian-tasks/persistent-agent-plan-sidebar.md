---
id: persistent-agent-plan-sidebar
title: Persistent model plan and runtime status sidebar
status: done
priority: high
model_level: high
task_type: feature
parent_id: cozyphi-convenience-program
tags:
    - tui
    - agent
    - session
    - persistence
    - ux
    - mcp
    - tokens
acceptance_criteria:
    - The model can publish and update an ordered plan with pending, in-progress, and completed steps through a small validated tool interface.
    - 'The right sidebar renders a fixed immediate-runtime section at the top: current model/mode, context/token usage, MCP connection state, and relevant background activity; the work plan is rendered below.'
    - A long plan scrolls independently without moving the runtime section, keeps the active step visible when practical, and exposes clear keyboard navigation.
    - The sidebar width is user-resizable within validated minimum and maximum bounds, persists as a user preference, and safely yields to deterministic compact or hidden modes on narrow terminals.
    - The current plan is persisted with the session and restored before the resumed UI becomes interactive, without waiting for a new model turn; runtime stats populate immediately when their sources are available.
    - The sidebar updates smoothly while generation and tools continue and does not block input.
    - Invalid transitions, stale updates, persistence failures, cancellation, unavailable runtime data, and invalid dimensions are surfaced explicitly; no silent return, panic, data race, or goroutine leak.
    - Focused tests cover tool validation, state transitions, persistence/resume, concurrent updates, MCP/token snapshot mapping, section ordering, independent scrolling, resizing, preference restore, and responsive rendering.
verification_plan:
    - Run focused tests for the plan module, session persistence, tool registry/execution, runtime status aggregation, settings, and TUI controller/rendering.
    - Run race-enabled tests for packages involved in concurrent plan and runtime status updates.
    - Run make fmt-check, make lint, and make test because the feature crosses agent, persistence, MCP, settings, and TUI seams.
created_at: "2026-08-24T15:05:50.444923Z"
updated_at: "2026-08-24T15:54:27.132842Z"
---

## Body

Build the standard right-side runtime status column, not a standalone todo widget. The fixed upper section shows immediate runtime state: current model/mode, context/token usage, MCP server state, and relevant background activity. The persistent model-managed work plan is rendered below in an independently scrollable region. Users can resize the sidebar within safe bounds; the preference survives restarts while narrow terminals deterministically collapse or hide the column without losing the preferred width. The plan is durable session state updated through a dedicated validated model tool and restored before resumed UI interaction. Runtime facts come from authoritative snapshots rather than being reconstructed by the widget. Updates must be ordered, cancellation-safe, non-blocking, observable, and must never silently disappear.

## Acceptance Criteria

- The model can publish and update an ordered plan with pending, in-progress, and completed steps through a small validated tool interface.
- The right sidebar renders a fixed immediate-runtime section at the top: current model/mode, context/token usage, MCP connection state, and relevant background activity; the work plan is rendered below.
- A long plan scrolls independently without moving the runtime section, keeps the active step visible when practical, and exposes clear keyboard navigation.
- The sidebar width is user-resizable within validated minimum and maximum bounds, persists as a user preference, and safely yields to deterministic compact or hidden modes on narrow terminals.
- The current plan is persisted with the session and restored before the resumed UI becomes interactive, without waiting for a new model turn; runtime stats populate immediately when their sources are available.
- The sidebar updates smoothly while generation and tools continue and does not block input.
- Invalid transitions, stale updates, persistence failures, cancellation, unavailable runtime data, and invalid dimensions are surfaced explicitly; no silent return, panic, data race, or goroutine leak.
- Focused tests cover tool validation, state transitions, persistence/resume, concurrent updates, MCP/token snapshot mapping, section ordering, independent scrolling, resizing, preference restore, and responsive rendering.

## Verification Plan

1. Run focused tests for the plan module, session persistence, tool registry/execution, runtime status aggregation, settings, and TUI controller/rendering.
2. Run race-enabled tests for packages involved in concurrent plan and runtime status updates.
3. Run make fmt-check, make lint, and make test because the feature crosses agent, persistence, MCP, settings, and TUI seams.

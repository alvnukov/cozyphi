---
id: fix-iterator-consumer-stop-panic
title: Iterator consumer stop can panic during stream cancellation
status: done
priority: high
task_type: bug
parent_id: cozyphi-enterprise-code-review
tags:
    - reliability
    - iterator
    - cancellation
    - tui
acceptance_criteria:
    - Stopping Loop consumption during a provider stream error never panics
    - Stopping Loop consumption during a tool event prevents later yields and tool execution
    - Compaction paths never yield after the consumer stops
    - All iterator producers in the repository honor yield=false
verification_plan:
    - Run focused consumer-stop regression tests in internal/agent
    - Run internal/agent and streaming package tests
    - Run make fmt-check lint test
created_at: "2026-08-24T10:45:29.397837Z"
updated_at: "2026-08-24T10:51:42.62016Z"
---

## Body

Esc or another consumer stop returns false from the agent event iterator. streamTurn, tool execution, and compaction paths can call yield again, causing Go 1.26 to panic with 'range function continued iteration after function for loop body returned false'.

## Acceptance Criteria

- Stopping Loop consumption during a provider stream error never panics
- Stopping Loop consumption during a tool event prevents later yields and tool execution
- Compaction paths never yield after the consumer stops
- All iterator producers in the repository honor yield=false

## Verification Plan

1. Run focused consumer-stop regression tests in internal/agent
2. Run internal/agent and streaming package tests
3. Run make fmt-check lint test

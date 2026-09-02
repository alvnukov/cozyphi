---
id: bound-mcp-framing-and-output
title: Bound MCP framing, parser depth, and retained output
status: done
priority: high
model_level: high
task_type: bug
parent_id: harness-security-hardening
tags:
    - security
    - mcp
    - dos
    - reliability
acceptance_criteria:
    - Oversized or deeply nested MCP frames fail before unbounded allocation.
    - A framing violation closes/resets the transport without response desynchronization.
    - stderr/log retention is bounded.
    - Errors identify the server and violated limit without echoing secrets.
verification_plan:
    - Hostile MCP fixture sends unterminated, oversized, deeply nested, and notification-flood inputs; memory remains bounded and next session recovers.
created_at: "2026-08-24T13:20:17.83815Z"
updated_at: "2026-09-02T19:01:40.122147Z"
---

## Body

internal/mcp/stdio.go uses bufio.Reader.ReadBytes until newline with no frame limit, allowing a server to exhaust memory; stderr can grow on disk without a per-server bound. Apply hard frame/body/depth limits with actionable errors, terminate/desync-reset the compromised transport, and bound retained logs.

## Acceptance Criteria

- Oversized or deeply nested MCP frames fail before unbounded allocation.
- A framing violation closes/resets the transport without response desynchronization.
- stderr/log retention is bounded.
- Errors identify the server and violated limit without echoing secrets.

## Verification Plan

1. Hostile MCP fixture sends unterminated, oversized, deeply nested, and notification-flood inputs; memory remains bounded and next session recovers.

---
id: fix-mcp-http-response-id-match
title: 'MCP http: responses not matched by request ID; SSE prefix brittle'
status: done
priority: medium
task_type: bug
parent_id: cozyphi-enterprise-code-review
tags:
    - reliability
    - mcp
    - review-2026-08
created_at: "2026-08-27T16:09:20.804026Z"
updated_at: "2026-08-27T21:26:28.414558Z"
---

## Body

internal/mcp/rpc.go:94-111 returns the FIRST parseable 'data: ' line, so a progress notification preceding the response is returned as the answer; 'data:' without a space is skipped. http.go:91-95 passes whatever arrives to resultOrError. Fix: scan all data lines and select the one whose ID matches; reuse util.ParseDataStream. Sibling of fix-mcp-stdio-desync-leak (same class, http transport).

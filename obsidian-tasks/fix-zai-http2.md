---
id: fix-zai-http2
title: Fix Z.AI Coding Plan TLS handshake timeouts
status: done
priority: high
task_type: bug
tags:
    - provider
    - llm
    - network
created_at: "2026-08-25T03:00:00.000000Z"
updated_at: "2026-08-25T03:20:00.000000Z"
---

## Body

Compaction (and any single-shot request) against the Z.AI Coding Plan
provider failed with `Post https://api.z.ai/api/coding/paas/v4/chat/completions:
net/http: TLS handshake timeout`. The shared HTTP client forced HTTP/1.1-only
(`ForceAttemptHTTP2=false`, ALPN `http/1.1`); `api.z.ai` stalls or drops the
TLS handshake for HTTP/1.1 clients, while HTTP/2 negotiates reliably.

Keep HTTP/2 enabled in `internal/util/httpclient.go` and add a regression
test asserting the shared transport advertises `h2`.

source_repo_path: /Users/zol/src/cozyphi

resolved_by: c59f97c fix(provider): negotiate HTTP/2 for z.ai handshakes

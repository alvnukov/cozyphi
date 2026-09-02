---
id: fix-zai-transport-effort
title: Fix Z.AI Coding Plan transport timeouts and add reasoning-effort levels
status: done
priority: high
task_type: bug
tags:
    - provider
    - llm
    - network
created_at: "2026-08-25T04:00:00.000000Z"
updated_at: "2026-08-25T04:40:00.000000Z"
---

## Body

Z.AI Coding Plan requests still failed with
`Post "https://api.z.ai/api/coding/paas/v4/chat/completions": net/http: TLS
handshake timeout` even after HTTP/2 negotiation was restored. The
`api.z.ai` edge intermittently drops TLS/connect attempts (curl showed
~70% failure on both HTTP/1.1 and HTTP/2), while the official ZCode host
`zcode.z.ai` is stable — that is why the ZCode client does not see the
problem: it calls a different frontend with JWT auth.

Fix: retry transient transport errors (TLS/dial timeouts, connection
resets, aborted/refused connections, truncated responses) in
`util.DoWithRetry`, honoring context cancellation.

Also add reasoning-effort levels for the Z.AI Coding Plan: GLM-5.x models
now expose `:minimal`/`:low`/`:medium`/`:high` variants through the model
switcher, and the OpenAI client sends `reasoning_effort` in the request
body (documented for GLM-5.2+; GLM-4.x gets no variants).

source_repo_path: /Users/zol/src/cozyphi

resolved_by: 73a1f3d fix(provider): retry z.ai transport failures, add reasoning effort

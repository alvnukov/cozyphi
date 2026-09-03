---
id: fix-llm-clients-parity-bounds
title: 'LLM clients: unbounded body reads, responses-client drift, accumulator clamp, usage growth'
status: done
priority: medium
task_type: bug
parent_id: cozyphi-enterprise-code-review
tags:
    - reliability
    - llm
    - review-2026-08
    - sector:agent-context
created_at: "2026-08-27T16:09:20.822547Z"
updated_at: "2026-08-27T23:27:35.545713Z"
---

## Body

1) io.ReadAll without cap on error bodies: anthropic/client.go:263,487 and openai/client.go:173,228 - responses/client.go:139 already caps via maxErrorBytes; a hostile endpoint OOMs the harness. 2) responses.Stream uses plain httpClient.Do (no transient/429 retry) and never calls cfg.Authenticator.Authorize, and errors drop the body (no llm.APIError) - one shared send+classify step for all three clients. 3) openai accumulator allocates by stream-controlled index (accum.go:99-118) - clamp. 4) tool-history repair runs twice on anthropic path (client/client.go:40 + anthropic/client.go:84) - repair once in Client.Stream. 5) internal/usage/store.go:154-163 score never decays frequency and Record never evicts - file only grows; decay or zero past recency window, prune stale entries.

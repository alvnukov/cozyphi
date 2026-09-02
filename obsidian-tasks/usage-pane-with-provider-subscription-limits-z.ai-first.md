---
id: usage-pane-with-provider-subscription-limits-z.ai-first
title: usage pane with provider subscription limits (z.ai first)
status: in_progress
priority: medium
model_level: high
task_type: feature
tags:
    - tui
    - providers
acceptance_criteria:
    - /usage opens a full-screen pane (Esc closes, r refreshes) with Subscription and Session sections
    - 'zai-coding-plan shows real quota windows: 5h session, weekly/monthly tokens, monthly reset, plan name'
    - providers without a quota adapter show an actionable 'not supported' message; failures surface as pane errors, never panic or block the editor
    - quota endpoint host is derived from the pinned credential BaseURL, not catalog data; the API key never reaches logs or transcripts
    - make fmt-check lint test green; CHANGELOG Unreleased line added
verification_plan:
    - make fmt-check
    - make lint
    - make test
    - 'manual: /usage with a configured zai-coding-plan provider'
created_at: "2026-08-30T16:13:55.001992Z"
updated_at: "2026-08-30T16:30:35.026535Z"
---

## Body

Claude Code style **/usage** panel for cozyphi, backed by provider subscription endpoints ("ручки"). Phase 1 targets the **zai-coding-plan** provider (API-key auth); the provider seam must accept further adapters (codex, anthropic) later.

Verified upstream contract (zai_status.py, same endpoint as CodexBar):
- GET https://api.z.ai/api/monitor/usage/quota/limit (CN region: https://open.bigmodel.cn), headers Authorization: Bearer <stored key>, Accept: application/json
- Response: {success, code:200, msg, data:{planName, limits:[{type: TOKENS_LIMIT|TIME_LIMIT, unit: 1=day/3=hour/5=minute/6=week, number, usage, currentValue, remaining, percentage, nextResetTime epoch ms}]}}
- shortest TOKENS_LIMIT window = 5h session cap, longest = monthly cap; TIME_LIMIT unit=minute number=1 is the monthly reset sentinel

Panel sections: **Subscription** (plan name, per-limit bars used/remaining + reset time) and **Session** (summed tokens in/out/cache/total from the session snapshot, rounds, wall duration, context fill). No USD cost and no lines added/removed — no data source exists yet. Endpoint host must derive from the pinned credential BaseURL, never from the remote catalog. Unsupported providers get a typed error and a clear pane message.

## Acceptance Criteria

- /usage opens a full-screen pane (Esc closes, r refreshes) with Subscription and Session sections
- zai-coding-plan shows real quota windows: 5h session, weekly/monthly tokens, monthly reset, plan name
- providers without a quota adapter show an actionable 'not supported' message; failures surface as pane errors, never panic or block the editor
- quota endpoint host is derived from the pinned credential BaseURL, not catalog data; the API key never reaches logs or transcripts
- make fmt-check lint test green; CHANGELOG Unreleased line added

## Verification Plan

1. make fmt-check
2. make lint
3. make test
4. manual: /usage with a configured zai-coding-plan provider

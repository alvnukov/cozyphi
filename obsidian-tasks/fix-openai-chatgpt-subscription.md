---
id: fix-openai-chatgpt-subscription
title: Implement OpenAI ChatGPT subscription login correctly
status: in_progress
priority: critical
task_type: bug
tags:
    - providers
    - openai
    - oauth
    - connect
    - security
acceptance_criteria:
    - /connect exposes OpenAI with ChatGPT Pro/Plus browser login as the primary method
    - Headless device authorization is an explicit fallback and API key remains a distinct OpenAI option
    - OAuth uses PKCE, random state, loopback callback validation, pinned Codex Responses endpoint, account id headers, and safe token refresh
    - No standalone misleading Codex provider remains in the catalog
    - Cancellation and failures are visible and do not block TUI input
verification_plan:
    - Add focused tests for browser PKCE URL, callback state/CSRF rejection, token exchange, cancellation, and OpenAI auth method selection
    - Run provider and TUI connect tests, then fmt-check lint test before commit
created_at: "2026-08-24T19:42:41.616132Z"
updated_at: "2026-08-24T19:42:41.616132Z"
---

## Body

Replace the mistakenly productized Codex-only device provider with OpenAI authentication methods matching the checked-in OpenCode implementation: browser Authorization Code + PKCE as the primary ChatGPT Pro/Plus subscription flow, headless device code as fallback, and API key as a separate OpenAI auth method. Keep OAuth endpoints pinned, preserve z.ai, never expose credentials, and keep TUI responsive/cancellable.

## Acceptance Criteria

- /connect exposes OpenAI with ChatGPT Pro/Plus browser login as the primary method
- Headless device authorization is an explicit fallback and API key remains a distinct OpenAI option
- OAuth uses PKCE, random state, loopback callback validation, pinned Codex Responses endpoint, account id headers, and safe token refresh
- No standalone misleading Codex provider remains in the catalog
- Cancellation and failures are visible and do not block TUI input

## Verification Plan

1. Add focused tests for browser PKCE URL, callback state/CSRF rejection, token exchange, cancellation, and OpenAI auth method selection
2. Run provider and TUI connect tests, then fmt-check lint test before commit

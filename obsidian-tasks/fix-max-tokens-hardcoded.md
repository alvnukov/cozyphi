---
id: fix-max-tokens-hardcoded
title: Model output token limit is hard-coded at 4096 and truncation is silent
status: done
tags:
    - bug
    - llm
    - ux
created_at: "2026-08-24T12:33:16.401858Z"
updated_at: "2026-08-24T12:48:25.481874Z"
---

## Body

Observed live: deepseek round 2026-08-24 14:50 ended with completion_tokens=4096 (hard cap), 16.5k chars reasoning, 0 chars answer — "model thought and thought but produced nothing". Root causes: (1) internal/llm/anthropic/client.go hard-codes defaultMaxTokens=4096; (2) neither llm client parses provider finish_reason, so session.StopMaxTokens is unreachable; (3) engine.go always infers StopEndTurn/StopToolUse; (4) transcript has no surface for a truncated round — silent failure.

Fix: add llm.ModelConfig.MaxOutputTokens + models[].max_output_tokens in config.yaml (0 = unset); anthropic request uses it with an 8192 fallback (API requires the field), openai sends max_tokens only when set; parse stop_reason/finish_reason in both clients onto llm.Choice.FinishReason; engine maps max_tokens/length to session.StopMaxTokens; session projection flags TurnMeta.Truncated and emits a visible warning row when a round is truncated before any text; formatTurnMeta appends "hit max tokens" to the turn footer. Mirror the new key in cmd/config.go web editor so saving config does not drop it.

Verification: red/green unit tests in llm/anthropic, llm/openai, project, session, transcript, agent; full make gate; CHANGELOG.

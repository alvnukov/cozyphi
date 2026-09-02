---
id: connect-provider-catalog
title: Add secure provider catalog and /connect workflow
status: in_progress
priority: high
model_level: high
task_type: feature
tags:
    - providers
    - connect
    - security
    - tui
    - models-dev
acceptance_criteria:
    - /connect opens a masked credential workflow and never places secrets in prompt history, session transcripts, logs, or config.yaml
    - Provider and model metadata refreshes from a strictly validated bounded catalog with atomic last-known-good fallback; remote updates cannot silently change trusted credential endpoints
    - Connected provider models become available to /model without restarting, while existing models[] configuration remains compatible
    - Provider protocol selection is explicit for supported transports and does not rely on model-name or URL heuristics
    - Cancellation and network failures are visible, actionable, and do not block the TUI event loop
verification_plan:
    - Provider module tests cover validation, last-known-good fallback, credential permissions, endpoint trust, and resolution
    - TUI command/overlay tests cover masked input, cancellation, dynamic models, and visible errors
    - Run affected package tests, race-sensitive tests where applicable, then make fmt-check lint test
created_at: "2026-08-24T18:24:31.100879Z"
updated_at: "2026-08-24T18:24:48.88843Z"
---

## Body

Introduce a deep provider module separating catalog metadata, credentials, connection workflow, and LLM transport selection. Deliver a secure non-blocking /connect vertical slice for API-key providers while preserving existing config compatibility. Synchronize provider/model metadata from models.dev through a validated last-known-good cache; never let untrusted catalog updates silently redirect credentials.

## Acceptance Criteria

- /connect opens a masked credential workflow and never places secrets in prompt history, session transcripts, logs, or config.yaml
- Provider and model metadata refreshes from a strictly validated bounded catalog with atomic last-known-good fallback; remote updates cannot silently change trusted credential endpoints
- Connected provider models become available to /model without restarting, while existing models[] configuration remains compatible
- Provider protocol selection is explicit for supported transports and does not rely on model-name or URL heuristics
- Cancellation and network failures are visible, actionable, and do not block the TUI event loop

## Verification Plan

1. Provider module tests cover validation, last-known-good fallback, credential permissions, endpoint trust, and resolution
2. TUI command/overlay tests cover masked input, cancellation, dynamic models, and visible errors
3. Run affected package tests, race-sensitive tests where applicable, then make fmt-check lint test

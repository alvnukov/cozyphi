---
id: connect-provider-catalog
title: Add secure provider catalog and /connect workflow
status: done
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
updated_at: "2026-09-03T07:22:26.311572Z"
---

## Body

Introduce a deep provider module separating catalog metadata, credentials, connection workflow, and LLM transport selection. Deliver a secure non-blocking /connect vertical slice for API-key providers while preserving existing config compatibility. Synchronize provider/model metadata from models.dev through a validated last-known-good cache; never let untrusted catalog updates silently redirect credentials.

**Done (2026-09-03).** Closed after an AC-by-AC audit on 2026-09-03 found no gaps: the work had landed earlier across commits but the ledger entry stayed open. Evidence per AC: (1) masked /connect overlay with paste masking, Esc wiping the secret buffer (internal/tui/overlays/connect.go + tests), ConnectRequest.APIKey zeroed after connect, ProviderConnectResultMsg carries no secret, credentials in a dedicated 0600 file written atomically (internal/provider/storage.go), never config.yaml; (2) models.dev catalog fetched with redirects disallowed, strictly decoded with bounds (maxCatalogBytes/maxProviders/maxModels/maxStringBytes), installed atomically via ReplaceCatalog, failure keeps the last-known-good cache (TestManagerRefreshKeepsLastKnownGoodCatalog), mergeBuiltins pins auth/endpoint/protocol so remote updates cannot redirect credentials, Connect matches ExpectedBaseURL; (3) connected models reach /model live via refreshModelCommands on ProviderConnectResultMsg/ProviderModelsUpdatedMsg, models[] config compatibility covered by controller_agent_model_test.go; (4) protocol explicit as enum-validated AuthMethod.Protocol/credential.Protocol (openai/openai-responses/anthropic only); (5) all network flows in goroutines with ctx timeouts and cancelAuth, failures surface as actionable toasts. Verification: go test ./internal/provider/... ./internal/tui/overlays/... ./internal/tui/controller/... all ok (2026-09-03, TEST_EXIT=0).

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

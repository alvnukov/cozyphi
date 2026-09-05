---
id: model-specific-openai-codex-efforts
title: Use model-specific effort capabilities for OpenAI and Codex
status: todo
priority: medium
model_level: medium
task_type: bug
tags:
    - openai
    - codex
    - reasoning
acceptance_criteria:
    - Resolve effort choices from the model/provider contract rather than one fixed four-level list.
    - Keep provider default distinct from explicit effort levels; verify xhigh availability for models that support it.
    - Document verified Codex UI labels separately from wire values; do not infer ultra equals max.
verification_plan:
    - Verify supported levels against model metadata or an authoritative provider source.
    - Add catalog and picker regression tests covering xhigh and provider default.
    - Check the selected effort reaches the Responses request unchanged.
created_at: "2026-09-05T14:57:59.248022Z"
updated_at: "2026-09-05T14:57:59.248022Z"
---

## Body

Read-only investigation at 37c3a4a found internal/provider/manager.go:160-168 hardcodes minimal, low, medium, high; Models at 521-523 applies this same list to OpenAI Responses subscription models. internal/llm/types.go already parses xhigh and max, and internal/llm/responses/client.go:235-240 forwards parsed values. Controller.ModelEfforts returns only the advertised list, so xhigh cannot be selected through the built-in catalog. The picker adds default, yielding five menu entries but only four explicit levels. Existing TestManagerFillsSubscriptionReasoningEfforts codifies the restricted list. User reports external Codex labels light, medium, high, very high, ultra; their exact mapping remains unverified. No implementation changes or runtime tests performed.

## Acceptance Criteria

- Resolve effort choices from the model/provider contract rather than one fixed four-level list.
- Keep provider default distinct from explicit effort levels; verify xhigh availability for models that support it.
- Document verified Codex UI labels separately from wire values; do not infer ultra equals max.

## Verification Plan

1. Verify supported levels against model metadata or an authoritative provider source.
2. Add catalog and picker regression tests covering xhigh and provider default.
3. Check the selected effort reaches the Responses request unchanged.

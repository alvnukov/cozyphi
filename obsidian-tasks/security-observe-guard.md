---
id: security-observe-guard
title: 05 — Observe bounded guard signals without secret persistence
status: blocked
priority: high
model_level: high
task_type: feature
parent_id: harness-security-hardening
tags: [security, guard, observability]
acceptance_criteria:
  - A tool-less isolated guard checks minimal permitted material and proposed actions; malformed or incomplete output is unknown.
  - Raw secrets reach only an explicitly warned and accepted local/own provider-endpoint-model guard, without persistent raw artifacts.
  - Observe records model suspicion without blocking solely on that signal; deterministic policy stays active.
  - Quarantine is bounded nonpersistent material or a restricted existing-source reference until encrypted storage exists.
  - Queue, input, output, concurrency, time, retries and cache retention are bounded; cached checks do not cache grants.
  - Audit contains safe local metadata only; off emits none and cancellation prevents stale results from applying.
verification_plan:
  - Exercise S08, S10 and S15 with scripted guard results and synthetic secret canaries in every captured output/storage sink.
  - Test invalid JSON, unexpected tool calls, truncation, timeout, redacted-input limits, cache context changes and queue cancellation.
  - Verify observations through public policy/engine behavior and UI/headless reasons; run only scoped tests and one scoped lint.
created_at: "2026-09-09T11:16:39Z"
updated_at: "2026-09-09T11:16:39Z"
---

## Body

**What to build:** A user sees inexpensive guard observations and potential stops on a protected task, with bounded processing and no automatic enforcement or strong-model call. A user may explicitly designate a local/own isolated guard as trusted after the full secret-access/server-logging warning.

**Blocked by:** security-model-egress, enforce-agent-dataflow-policy.

**Contract:** [Specification](../specs/harness-security.md), D6–D7; slice 05 of the [approved map](../specs/harness-security-tickets.md).

**Scope:** Guard profile acceptance, minimal input, structured reason/position output, safe observation/quarantine metadata and local audit. Trust is identity-bound and initially guard-only. No parent history, executable tools, recursive checking or raw secrets in error/debug/temp/job output. Lack of trusted processing yields limited operation or pause, not external secret disclosure. Model selection remains an evidence decision; develop with fake providers unless separately authorized.

**Blocked (2026-09-09):** Waiting for model egress and action policy so observation cannot create its own bypass.

## Acceptance Criteria

- A tool-less isolated guard checks minimal permitted material and proposed actions; malformed or incomplete output is unknown.
- Raw secrets reach only an explicitly warned and accepted local/own provider-endpoint-model guard, without persistent raw artifacts.
- Observe records model suspicion without blocking solely on that signal; deterministic policy stays active.
- Quarantine is bounded nonpersistent material or a restricted existing-source reference until encrypted storage exists.
- Queue, input, output, concurrency, time, retries and cache retention are bounded; cached checks do not cache grants.
- Audit contains safe local metadata only; off emits none and cancellation prevents stale results from applying.

## Verification Plan

1. Exercise S08, S10 and S15 with scripted guard results and synthetic secret canaries in every captured output/storage sink.
2. Test invalid JSON, unexpected tool calls, truncation, timeout, redacted-input limits, cache context changes and queue cancellation.
3. Verify observations through public policy/engine behavior and UI/headless reasons; run only scoped tests and one scoped lint.

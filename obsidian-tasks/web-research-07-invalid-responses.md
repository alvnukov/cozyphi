---
id: web-research-07-invalid-responses
title: 07 — Reject malformed quarantine output and all attempted tool calls
status: blocked
priority: high
model_level: low
task_type: test
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Malformed, oversized, empty, truncated and unknown-status outputs fail closed in every quarantine stage.
    - Any tool-call event, including an unknown tool name, aborts without invoking an executor or forwarding arguments.
    - Errors, progress and logs expose only bounded safe reason codes, not hostile response bodies or synthetic secrets.
verification_plan:
    - Run the fixed stage-by-error matrix against the public tracer boundary, including hostile error/argument sentinels.
    - Check parent output, safe progress and executor counters, not only returned error values.
    - Attach failing-before/passing-after evidence and scoped test commands; leave no fault-injection edits committed.
created_at: "2026-09-19T19:11:59.062677Z"
updated_at: "2026-09-19T19:11:59.062677Z"
---

## Body

**What to build:** A fixed negative-response matrix around the single-source tracer so incomplete model output never becomes pass.

**Blocked by:** [06](web-research-06-single-source-tracer.md).

**Contract:** [Spec](../specs/protected-web-research.md), D5/D6/D9, Q27/Q28, T04/T05/T14. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Add table-driven public-boundary cases for invalid JSON, extra/unknown status, missing fields, empty body, truncation, provider error, timeout, oversized output and unexpected tool events. Run the same relevant cases at safety, extraction and final screening. Preserve the bounded outcome vocabulary established by 06. Small decoder/error-path fixes are allowed; architecture changes are not. Include malicious strings in free-text errors and tool arguments to test non-delivery.

**Do not change:** No retries-until-pass, schema-coercion guesses, live provider calls or detector-quality claims.

**Proof required:** Named cases and public result/provider/executor counters for each stage; zero unchecked payload deliveries and zero executor invocations. The regression must fail when the corresponding rejection branch is deliberately bypassed in a temporary test experiment, then restored.

**Stop condition:** If failures require a new shared lifecycle design, report that gap rather than broaden this low-level task.

## Acceptance Criteria

- Malformed, oversized, empty, truncated and unknown-status outputs fail closed in every quarantine stage.
- Any tool-call event, including an unknown tool name, aborts without invoking an executor or forwarding arguments.
- Errors, progress and logs expose only bounded safe reason codes, not hostile response bodies or synthetic secrets.

## Verification Plan

1. Run the fixed stage-by-error matrix against the public tracer boundary, including hostile error/argument sentinels.
2. Check parent output, safe progress and executor counters, not only returned error values.
3. Attach failing-before/passing-after evidence and scoped test commands; leave no fault-injection edits committed.

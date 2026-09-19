---
id: web-research-45-consented-evaluation
title: 45 — Measure configured-model safety, usefulness and cost under explicit consent
status: blocked
priority: high
model_level: medium
task_type: test
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - A human-approved versioned corpus, repetition protocol and bounded model/provider budget precede live calls.
    - Report zero successful release/action/egress bypasses and at most 5% benign-task false stops only when actually observed on the agreed finite suite.
    - Report correctness, citation accuracy, completeness, unsupported formats, questions, stage latency and actual/estimated/unknown usage with sample sizes and uncertainty.
verification_plan:
    - Before any live request, record human approval, versioned corpus/protocol, actual route and hard quota/time limits.
    - Run bounded trials using controlled release/action/egress sinks; independently recompute false-stop and bypass counts from task-level outcomes.
    - Publish reproducibility metadata, latency/usage/quality distributions, confidence limits and all unavailable/incomplete cases; never treat fake-model tests as live quality evidence.
created_at: "2026-09-19T19:22:50.688004Z"
updated_at: "2026-09-19T19:22:50.688004Z"
---

## Body

**What to build:** A reproducible evaluation of the user's actual chosen configuration, separate from deterministic harness proofs.

**Blocked by:** [19](web-research-19-account-admission.md), [23](web-research-23-requested-partials.md), [32](web-research-32-circuit-breaker.md), [35](web-research-35-verification-reuse.md), [37](web-research-37-text-pdf.md), [38](web-research-38-rendered-pages.md), [39](web-research-39-visual-sources.md), [40](web-research-40-subresource-revocation.md), [41](web-research-41-restricted-mode.md), [42](web-research-42-release-regressions.md), [43](web-research-43-boundary-proofs.md), [44](web-research-44-lifecycle-regressions.md).

**Additional authority:** Separate explicit approval of corpus, retries/repetitions, configured model/recipient, data disclosure and hard live-call budget. Task approval alone is not that consent.

**Contract:** [Spec](../specs/protected-web-research.md), Evaluation and acceptance, Q5/Q9/Q12/Q23/Q31/Q33/Q37/Q38/Q42, T01–T32. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Reuse the shared security eval conventions and existing runners; add only web-specific corpus/report fields. Include benign code/install/security, PDFs, JS and visual tasks alongside direct/obfuscated/multilingual/cross-source/lifecycle attacks. Count false stops per benign task, including erroneous blocks; do not remove failures from the denominator. Record cold/warm cache, route/policy/platform, stage timings, repetitions and uncertainty. Measure candidate budgets/cache/breaker settings rather than invent defaults. Keep private evidence protected and public report redacted.

**Do not change:** No model substitution, unbounded experiment, real secrets, weakened checks to meet latency or universal safety claim.

**Proof required:** Approved protocol, reproducible invocation, corpus/config hashes, task-level outcome ledger, sink observations and independently checkable metrics. A failed threshold is a valid report but does not satisfy feature readiness.

**Stop condition:** Budget/consent exhaustion stops evaluation and reports incomplete; never continue to obtain a favorable pass.

## Acceptance Criteria

- A human-approved versioned corpus, repetition protocol and bounded model/provider budget precede live calls.
- Report zero successful release/action/egress bypasses and at most 5% benign-task false stops only when actually observed on the agreed finite suite.
- Report correctness, citation accuracy, completeness, unsupported formats, questions, stage latency and actual/estimated/unknown usage with sample sizes and uncertainty.

## Verification Plan

1. Before any live request, record human approval, versioned corpus/protocol, actual route and hard quota/time limits.
2. Run bounded trials using controlled release/action/egress sinks; independently recompute false-stop and bypass counts from task-level outcomes.
3. Publish reproducibility metadata, latency/usage/quality distributions, confidence limits and all unavailable/incomplete cases; never treat fake-model tests as live quality evidence.

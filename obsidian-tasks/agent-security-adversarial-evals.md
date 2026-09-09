---
id: agent-security-adversarial-evals
title: 10 — Measure harness attacks and benign task outcomes reproducibly
status: todo
priority: high
model_level: high
task_type: test
parent_id: harness-security-hardening
tags:
    - security
    - evals
    - prompt-injection
    - regression
acceptance_criteria:
    - A versioned offline corpus and controlled sinks measure actual forbidden disclosures/actions across every specified ingress and lifecycle.
    - Deterministic control-plane regressions run in CI on relevant changes independently of network models and preserve previously fixed exploit chains.
    - Benign task completion and task-level false stops are separate from fragment labels and observe-mode potential stops.
    - Model-backed reports identify model/version, repeated attempts, sample sizes, uncertainty, coverage, p50/p95, cost and questions/escalations.
    - Attack thresholds and model choice remain explicit human readiness decisions; no live or paid trials run without separate approval.
verification_plan:
    - Exercise S16 with synthetic canaries, fake providers and controlled sinks; verify secret-safe UI/headless reports distinguish unknown, potential stops and actual outcomes.
    - Cover multilingual, long, encoded, quiet-poisoning, authority-forgery, compaction, child, hooks/MCP and retry attacks plus representative benign tasks.
    - Run only changed-scope deterministic checks locally; publish model trial commands/results only for an explicitly authorized run.
created_at: "2026-08-24T13:20:17.843656Z"
updated_at: "2026-09-09T11:16:39Z"
---

## Body

**What to build:** A maintainer can reproduce a forbidden disclosure/action and a benign task outcome at controlled boundaries, compare modes and report model-dependent evidence without making normal tests depend on live providers. This expands the existing eval task, not a second corpus owner.

**Blocked by:** None — can start immediately with an offline corpus. Feature slices add their own regression cases as they land; completed enforcement is not required to build the evaluation interface.

**Contract:** [Specification](../specs/harness-security.md), D9 and S01–S17; slice 10 of the [approved map](../specs/harness-security-tickets.md).

**Scope:** Repository/tool/schema/config injections, exfiltration, approval bypass, recursive retries, multi-agent propagation and supply-chain hooks/MCP. Evaluate outcomes rather than detector labels. Target at most 5% false stops per benign task and p95 ordinary/strong check latency of one/five seconds including queue/network, excluding human waiting. These are goals; attack-success threshold, model choice, sample-size policy and monetary budget are unresolved. No historical score substitutes for this report. Runtime enforcement readiness and full V1 walkthrough remain slices 11 and 12.

## Acceptance Criteria

- A versioned offline corpus and controlled sinks measure actual forbidden disclosures/actions across every specified ingress and lifecycle.
- Deterministic control-plane regressions run in CI on relevant changes independently of network models and preserve previously fixed exploit chains.
- Benign task completion and task-level false stops are separate from fragment labels and observe-mode potential stops.
- Model-backed reports identify model/version, repeated attempts, sample sizes, uncertainty, coverage, p50/p95, cost and questions/escalations.
- Attack thresholds and model choice remain explicit human readiness decisions; no live or paid trials run without separate approval.

## Verification Plan

1. Exercise S16 with synthetic canaries, fake providers and controlled sinks; verify secret-safe UI/headless reports distinguish unknown, potential stops and actual outcomes.
2. Cover multilingual, long, encoded, quiet-poisoning, authority-forgery, compaction, child, hooks/MCP and retry attacks plus representative benign tasks.
3. Run only changed-scope deterministic checks locally; publish model trial commands/results only for an explicitly authorized run.

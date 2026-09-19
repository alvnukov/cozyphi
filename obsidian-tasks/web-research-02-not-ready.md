---
id: web-research-02-not-ready
title: 02 — Refuse unready protected web without legacy bypasses
status: in_progress
priority: high
model_level: low
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
    - ready-for-agent
branch: feature/web-research-02-not-ready
worktree_path: .worktrees/web-research-02-not-ready
acceptance_criteria:
    - Missing protected readiness returns an actionable refusal with zero acquisition/model calls.
    - Legacy raw, disabled quarantine, missing reader and general allow-all cannot release unchecked web content.
    - No protected-ready claim is emitted merely because legacy web.enabled is true.
verification_plan:
    - Exercise public web calls with missing binding, raw, quarantine-off, missing reader and general off/observe/allow-all.
    - Capture model-facing outcomes and fake provider/network counters; assert safe refusal and no calls.
    - Run changed-package tests and scoped formatting/build; attach exact commands and regression evidence per delivery rules.
created_at: "2026-09-19T19:10:37.683088Z"
updated_at: "2026-09-19T22:01:29.455917Z"
---

## Body

**What to build:** A fail-closed migration boundary: incomplete protected-web setup cannot silently use the old raw or session-model paths.

**Blocked by:** None — can start immediately.

**Contract:** [Spec](../specs/protected-web-research.md), D2/D3/D7/D16, Q9/Q13/Q25, T01/T02/T13. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Trace existing web entry points and configuration. Add one host-owned not-ready outcome with safe setup guidance. Reject legacy raw/quarantine-off delivery and missing-reader fallback at the actual public boundary, including direct search snippet delivery. Keep protected research unavailable until later tickets provide required capabilities. Test legacy configuration decoding without treating it as authorization. Explain changed behavior in user-facing docs/changelog.

**Do not change:** Do not implement model binding, research orchestration, sandbox or a bypass flag. Do not weaken protection to retain old behavior.

**Proof required:** Public tool/session request captures for missing configuration, raw request, quarantine-off and allow-all; zero provider/network invocations and zero unchecked source bytes in model output. Include a normal disabled-web control.

**Stop condition:** If another entry point cannot be gated within this narrow change, report it as a blocker instead of claiming migration complete.

**Started (2026-09-20).** Taking it after PR #42 merge (main c9833699). Branch feature/web-research-02-not-ready, worktree .worktrees/web-research-02-not-ready. Fail-closed migration boundary per spec D2/D3/D7/D16: not-ready refusal with setup guidance; legacy raw/quarantine-off/missing-reader/session-model paths refuse; zero provider/network calls on refusal paths.

## Acceptance Criteria

- Missing protected readiness returns an actionable refusal with zero acquisition/model calls.
- Legacy raw, disabled quarantine, missing reader and general allow-all cannot release unchecked web content.
- No protected-ready claim is emitted merely because legacy web.enabled is true.

## Verification Plan

1. Exercise public web calls with missing binding, raw, quarantine-off, missing reader and general off/observe/allow-all.
2. Capture model-facing outcomes and fake provider/network counters; assert safe refusal and no calls.
3. Run changed-package tests and scoped formatting/build; attach exact commands and regression evidence per delivery rules.

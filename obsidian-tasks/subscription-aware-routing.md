---
id: subscription-aware-routing
title: Route model quality tiers across subscription budgets
status: todo
priority: high
model_level: very_high
task_type: epic
tags:
    - routing
    - subscriptions
    - goal
acceptance_criteria:
    - Users explicitly opt into versioned tier semantics and assign one tier per verified model/native-effort profile; legacy artifacts retain their original meaning until approved migration.
    - Main and child routing preserves required quality, actual-tier awareness, user pins and security gates while coordinating account commitments across all local processes.
    - OpenAI OAuth/Codex and Z.AI adapters preserve evidence uncertainty and every applicable quota window; adaptive reserve and useful surplus respect the approved priority policy.
    - Risk exceptions require genuine once-per-scope human confirmation; paid routing defaults off and cannot claim an unenforceable cap.
    - All nine child slices pass their verification plans and the S01–S20 scenario matrix with disclosed evidence limitations.
verification_plan:
    - Work the dependency frontier documented in the approved breakdown; do not mark the epic complete when only the specification exists.
    - Run offline provider fixtures, public admission tests, cross-process races and engine lifecycle scenarios S01–S20.
    - Compare fixed baseline and adaptive traces; inspect quality/authority violations before savings, latency or expiry metrics.
    - Require separate consent for live verification and record unknown provider assumptions as rollout blockers or explicit limitations.
created_at: "2026-09-06T11:09:44.07522Z"
updated_at: "2026-09-06T11:09:44.07522Z"
---

## Body

**Contract:** [Subscription-aware quality-tier routing](../specs/subscription-aware-routing.md), with the [approved breakdown](../specs/subscription-aware-routing-tickets.md) and [source evidence](../specs/subscription-aware-routing-evidence.md).

**What to build:** Opt-in automatic model/native-effort selection by user-assigned quality tiers, sharing subscription commitments across local processes, reserving capacity for important work and spending credible surplus on already-assigned useful work. Required tier is not native effort. Reuse plan/spawn effort only under an explicit versioned interpretation.

**Blocked by:** None — container; implement the child frontier in dependency order. Slice 01 is gated by the existing model-specific-openai-codex-efforts task.

**Scope:** Nine approved vertical slices, existing OpenAI OAuth/Codex and Z.AI Coding Plan integrations, deterministic offline verification first. No new Claude backend, model-authored ranking, synthetic work to burn quota or implicit paid overage.

**Related, not blocking:** user-root-tool-override separately covers general human overrides of tool restrictions. executor-quality-evaluation and executor-context-budget retain their own approval contracts.

**Delivery state:** Specification only; no routing implementation or live provider verification has been performed.

## Acceptance Criteria

- Users explicitly opt into versioned tier semantics and assign one tier per verified model/native-effort profile; legacy artifacts retain their original meaning until approved migration.
- Main and child routing preserves required quality, actual-tier awareness, user pins and security gates while coordinating account commitments across all local processes.
- OpenAI OAuth/Codex and Z.AI adapters preserve evidence uncertainty and every applicable quota window; adaptive reserve and useful surplus respect the approved priority policy.
- Risk exceptions require genuine once-per-scope human confirmation; paid routing defaults off and cannot claim an unenforceable cap.
- All nine child slices pass their verification plans and the S01–S20 scenario matrix with disclosed evidence limitations.

## Verification Plan

1. Work the dependency frontier documented in the approved breakdown; do not mark the epic complete when only the specification exists.
2. Run offline provider fixtures, public admission tests, cross-process races and engine lifecycle scenarios S01–S20.
3. Compare fixed baseline and adaptive traces; inspect quality/authority violations before savings, latency or expiry metrics.
4. Require separate consent for live verification and record unknown provider assumptions as rollout blockers or explicit limitations.

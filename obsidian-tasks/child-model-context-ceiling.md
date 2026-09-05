---
id: child-model-context-ceiling
title: Preserve captured child context ceiling across model changes
status: done
priority: medium
model_level: medium
task_type: bug
tags:
    - agents
    - context
acceptance_criteria:
    - An interactive child's manual/plan model switch cannot widen its captured spawn context ceiling.
    - A smaller target model retains its own smaller window; parent/main sessions remain independently configurable.
verification_plan:
    - Reproduce with a child spawned under a lower context limit, then select a larger-window model.
    - Test manual and plan selection plus parent/sibling isolation.
created_at: "2026-09-05T21:50:52.140903Z"
updated_at: "2026-09-05T22:10:11.540442Z"
---

## Body

Static integration finding: EngineRunner.PrepareChild narrows only the initial Model.ContextWindow using Runner.ContextLimit. controller.newChild retains opts.Model but not the captured ceiling; later Controller model selection resolves a fresh catalog config and Engine.SetModel replaces contextWindow. This appears to predate active-turn model selection (the old idle swap also passed a fresh config). Add a focused reproduction and preserve the captured ceiling at the engine/model seam rather than reload mutable parent settings. Not part of the bounded selector closeout; no failing test claimed.

**Done (2026-09-06).** Commit b67e436, merge c9fa225 in main. Ceiling now lives on the engine: EngineOpts.ContextCeiling → Engine.contextCeiling (immutable), windowLocked() = effectiveWindow(effectiveWindow(model, ceiling), override) used by NewEngine, setModelLocked and SetContextWindowOverride. PrepareChild passes ContextLimit as ContextCeiling and leaves Model.ContextWindow honest. Tests: TestContextCeiling (spawn cap, unknown window, manual SelectModel to wider model, smaller model keeps own window, plan step pin + restore, session override cannot widen past ceiling, zero = none) and TestEngineRunnerContextLimitIsEngineCeiling. go test -race internal/agent + internal/tui/controller green; golangci-lint run internal/agent — 0 new (1 pre-existing perfsprint in untouched model_selection.go). CHANGELOG under [Unreleased]. Worktree and branch removed. No push.

## Acceptance Criteria

- An interactive child's manual/plan model switch cannot widen its captured spawn context ceiling.
- A smaller target model retains its own smaller window; parent/main sessions remain independently configurable.

## Verification Plan

1. Reproduce with a child spawned under a lower context limit, then select a larger-window model.
2. Test manual and plan selection plus parent/sibling isolation.

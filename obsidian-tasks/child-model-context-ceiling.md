---
id: child-model-context-ceiling
title: Preserve captured child context ceiling across model changes
status: todo
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
updated_at: "2026-09-05T21:50:52.140903Z"
---

## Body

Static integration finding: EngineRunner.PrepareChild narrows only the initial Model.ContextWindow using Runner.ContextLimit. controller.newChild retains opts.Model but not the captured ceiling; later Controller model selection resolves a fresh catalog config and Engine.SetModel replaces contextWindow. This appears to predate active-turn model selection (the old idle swap also passed a fresh config). Add a focused reproduction and preserve the captured ceiling at the engine/model seam rather than reload mutable parent settings. Not part of the bounded selector closeout; no failing test claimed.

## Acceptance Criteria

- An interactive child's manual/plan model switch cannot widen its captured spawn context ceiling.
- A smaller target model retains its own smaller window; parent/main sessions remain independently configurable.

## Verification Plan

1. Reproduce with a child spawned under a lower context limit, then select a larger-window model.
2. Test manual and plan selection plus parent/sibling isolation.

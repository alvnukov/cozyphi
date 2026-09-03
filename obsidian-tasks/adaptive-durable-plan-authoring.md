---
id: adaptive-durable-plan-authoring
title: Adaptive durable-plan authoring without harness classification
status: done
task_type: epic
tags:
    - epic
    - plan-v2
    - authoring
created_at: "2026-08-30T13:21:17.270995Z"
updated_at: "2026-08-30T14:55:23.186644Z"
---

## Body

**Problem:** the model that authors a durable plan gets type-permission prose but almost no guidance on how to shape the plan itself. Harness stays authoritative for gate, approval, lifecycle and evidence; authoring guidance belongs to the model, not a task classifier.

**Architecture contract (non-executable):** the model extracts obligations, workstreams, dependencies, uncertainty and evidence boundaries, then builds the smallest complete bespoke plan. Capability type = least sufficient cumulative tool set; semantic role lives in content/why/doneWhen, never in the type. Harness keeps objective limits: tool loop, gate/approval, lifecycle, JIT, evidence — no semantic validator. Archetype retrieval is a late advisory seam only; no-match is normal; nothing here implements it. Telemetry stays bounded and privacy-safe; never a semantic oracle or automatic policy source.

**Children (tracer bullets, model_level=medium):** adaptive-plan-authoring-grammar -> adaptive-plan-authoring-policy -> plan-authoring-telemetry -> adaptive-plan-authoring-integration; plan-step-supersede -> adaptive-plan-authoring-integration.

**Close condition:** epic stays blocked until all five children are done. Repo code must not change as part of the epic itself.

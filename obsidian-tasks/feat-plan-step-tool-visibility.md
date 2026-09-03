---
id: feat-plan-step-tool-visibility
title: 'Plan-gated tool visibility: model sees only tools the active steps permit'
status: done
priority: medium
task_type: feature
tags:
    - plangate
    - tools
    - context
    - useplan
created_at: "2026-08-27T22:44:42.035139Z"
updated_at: "2026-08-27T22:56:00.129665Z"
---

## Body

Hide tool schemas the current plan state does not permit: in useplan/deny the provider-visible tool list narrows to the exempt set plus the union of tools allowed by in_progress steps (rank inheritance). Drafting modes (hint) keep the full list. Executor plan gate stays as enforcement (defense in depth: hallucinated calls to hidden tools still deny with the standard reason).

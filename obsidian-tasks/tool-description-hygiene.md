---
id: tool-description-hygiene
title: Make tool descriptions and plan error labels match their schemas
status: done
priority: medium
model_level: medium
task_type: fix
tags:
    - tools
    - prompt
    - plan
acceptance_criteria:
    - Every model-facing parameter carries a description; task declares action required.
    - Plan evidence hint and session plan errors name evidenceRefs, noEvidenceReason, resumeWhen and planResult as the tool spells them.
    - edit items no longer advertise extra properties; memory does not introduce itself as a Claude Code tool.
    - question says when to ask, bounds questions and options, puts the recommended option first and tells the model the UI adds the free-text answer.
    - agent_spawn guidance says the user never sees a sub-agent summary and forbids predicting a pending result.
verification_plan:
    - go test ./internal/tools/... ./internal/session/
    - scoped golangci-lint run over internal/tools/... and internal/session/...
created_at: "2026-09-09T21:24:21.958487Z"
updated_at: "2026-09-09T21:27:00.696788Z"
---

## Body

**Origin.** Comparing cozyphi's tool definitions with the ones a Claude Code session log carries turned up places where the text the model reads disagrees with the schema it must satisfy, and text that names cozyphi's neighbours instead of itself.

**Findings.** `session.action` and `agent_cancel.job_id` had no description. `task` listed no required keys although action is mandatory. The plan `evidence` hint said `evidence_refs` / `no_evidence_reason` while the properties are `evidenceRefs` / `noEvidenceReason`, and the session plan-transition errors used the same snake names plus `resume_when` and `plan_result`, so a model that obeyed the error would send a field the strict decoder rejects. `edit.edits.items` carried `additionalProperties: true` against the strict-decode convention. `memory` introduced itself as the Claude Code auto memory. `question` gave no rule for when to ask, no bounds, no recommended-first convention and did not say the UI adds a free-text answer, so models invented an "other" option. `agent_spawn` guidance did not say the user never sees the summary nor forbid predicting a pending job's result, both of which Claude Code states.

**Change.** Text and schema only, no behaviour: descriptions, `required`, `minItems`/`maxItems` on question arrays, error labels, and the pinned tests.

## Acceptance Criteria

- Every model-facing parameter carries a description; task declares action required.
- Plan evidence hint and session plan errors name evidenceRefs, noEvidenceReason, resumeWhen and planResult as the tool spells them.
- edit items no longer advertise extra properties; memory does not introduce itself as a Claude Code tool.
- question says when to ask, bounds questions and options, puts the recommended option first and tells the model the UI adds the free-text answer.
- agent_spawn guidance says the user never sees a sub-agent summary and forbids predicting a pending result.

## Verification Plan

1. go test ./internal/tools/... ./internal/session/
2. scoped golangci-lint run over internal/tools/... and internal/session/...

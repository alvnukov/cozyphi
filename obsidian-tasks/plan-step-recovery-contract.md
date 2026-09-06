---
id: plan-step-recovery-contract
title: Preserve plan gate recovery hints and clarify the system protocol
status: done
priority: high
model_level: high
task_type: bug
tags:
    - plan
    - prompt
    - reliability
branch: bug/plan-step-recovery-contract
worktree_path: .worktrees/plan-step-recovery-contract
acceptance_criteria:
    - Denied tool calls expose both the plan gate reason and recovery hint without executing the tool.
    - The system prompt teaches the cozyphi-specific plan_step protocol, parallel child bindings, and correcting the binding rather than changing the tool.
    - Regression tests pass at the executor output and public PromptBlock seams; gate semantics remain unchanged.
verification_plan:
    - Exercise missing plan_step with multiple compatible steps; assert reason and hint reach model, handler remains blocked.
    - Check system instruction contract through PromptBlock, then existing plangate/executor regressions.
    - Format and run scoped gates, one informational full suite, separate Standards/Spec review against b5f4a0a.
created_at: "2026-09-06T10:09:55.133444Z"
updated_at: "2026-09-06T10:36:06.719712Z"
---

## Body

**Scope:** Deliver the recovery Hint currently dropped by executor on plan denial and make the system prompt's plan_step protocol operational. No model experiment, no gate/autobind semantics changes. Historical diagnosis confirmed seven omitted bindings and two successful corrected retries; prompt efficacy remains unproven.

**Verification boundaries:** User approved executor denial output and public PromptBlock, with review against initial main b5f4a0a.

**Note (2026-09-06).** Implemented model-only denial recovery hints and explicit host-tool binding/retry instructions in .worktrees/plan-step-recovery-contract. Both seam regressions passed after first reproducing the missing contract; existing PromptBlock tests retain the 4000-byte cap. Four modified/new Go files have fresh clean gopls diagnostics. Scoped race/build/lint and one informational full-suite run are in progress; Standards/Spec review and integration remain.

**Done (2026-09-06).** Landed in main via merge 6828351 (implementation a742bb2, public-loop regression 68b167e). Denials preserve model-only recovery hints; PromptBlock teaches explicit bindings and corrected retries. Permissions/autobinding unchanged. Build, fresh gopls diagnostics, scoped race tests passed; one informational go test ./... passed before the test-only public-loop revision, followed by passing agent/plangate race tests on that revision. Standards review found a private test boundary, corrected to Engine.Loop and verified; Spec review had no findings. Scoped lint found a new-test assertion style issue (fixed) and pre-existing perfsprint in unchanged internal/agent/model_selection.go:32; lint not rerun per scoped-gate policy. Concurrent main changes caused only a CHANGELOG conflict, resolved in task worktree preserving both entries; merged prompt diff reviewed. No model experiment or claim of prompt efficacy. No push.

## Acceptance Criteria

- Denied tool calls expose both the plan gate reason and recovery hint without executing the tool.
- The system prompt teaches the cozyphi-specific plan_step protocol, parallel child bindings, and correcting the binding rather than changing the tool.
- Regression tests pass at the executor output and public PromptBlock seams; gate semantics remain unchanged.

## Verification Plan

1. Exercise missing plan_step with multiple compatible steps; assert reason and hint reach model, handler remains blocked.
2. Check system instruction contract through PromptBlock, then existing plangate/executor regressions.
3. Format and run scoped gates, one informational full suite, separate Standards/Spec review against b5f4a0a.

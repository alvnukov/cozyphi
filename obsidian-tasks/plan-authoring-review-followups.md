---
id: plan-authoring-review-followups
title: Plan authoring review followups
status: done
task_type: issue
tags:
    - plan
    - plantel
    - code-review
acceptance_criteria:
    - plantel authoring-friction counters (DraftCreated, ApprovalLatency, MaterialReapproval, PatchRetry, CompletionSuccess, CompletionAbandoned) increment from production call sites, covered by wiring tests
    - Terminal statuses are defined once via session.PlanStatus.Terminal() and used at session/plan.go, plantool remainingSteps, sidebar drawPlanDivider, plangate projection
    - plangate.AuthoringPolicy is a closed string type carried through prompt.Options.PlanGrammar, engine systemPrompt, harnesssettings.Draft and settings pane; plantel.Policy renamed to AuthoringPolicy (constants AuthoringAdaptive/AuthoringLegacy)
    - doc/project-layout.md lists internal/plantel, internal/planscen and doc/plan-authoring.md; CHANGELOG.md [Unreleased] covers the fixes; doc/plan-authoring.md claims match reality
    - make fmt-check lint test green; conventional commits, one logical change each; main clean after merge
verification_plan:
    - go test ./internal/session/... ./internal/agent/... ./internal/plantel/... ./internal/plangate/... ./internal/tools/plantool/... ./internal/harnesssettings/... ./internal/tui/...
    - 'grep: no hand-enumerated completed/cancelled/superseded outside Terminal() and switch statements with distinct semantics'
    - 'grep: DraftCreated/ApprovalLatency/MaterialReapproval/PatchRetry/CompletionSuccess/CompletionAbandoned have production call sites outside plantel tests'
    - make fmt-check lint test in the worktree before merge to main
created_at: "2026-08-30T15:45:32.485817Z"
updated_at: "2026-08-30T16:49:01.489924Z"
---

## Body

**Problem:** the two-axis code-review of epic `adaptive-durable-plan-authoring` (bba6003..803175f) found the plantel authoring-friction counters dead code (zero production call sites, doc/plan-authoring.md:26-33 overclaims), the terminal-status set hand-enumerated in four behavior sites, the authoring_policy selector travelling as bare string, plantel.Policy colliding with plangate.Policy, and doc/project-layout.md missing plantel/planscen/plan-authoring.md.

**Scope:** internal/plantel (rename), internal/plangate (typed AuthoringPolicy), internal/agent (draft tag translation, prompt field), internal/session (wiring: approval latency, material reapproval, patch retry, completion outcome; PlanStatus.Terminal()), internal/tools/plantool + internal/tui/sidebar + internal/plangate/projection (Terminal() call sites), internal/harnesssettings + internal/tui/settings (typed selector), doc/project-layout.md, CHANGELOG.md.

**Out of scope:** archetype retrieval, new counters, cancellation semantics, legacy update path counting.

## Acceptance Criteria

- plantel authoring-friction counters (DraftCreated, ApprovalLatency, MaterialReapproval, PatchRetry, CompletionSuccess, CompletionAbandoned) increment from production call sites, covered by wiring tests
- Terminal statuses are defined once via session.PlanStatus.Terminal() and used at session/plan.go, plantool remainingSteps, sidebar drawPlanDivider, plangate projection
- plangate.AuthoringPolicy is a closed string type carried through prompt.Options.PlanGrammar, engine systemPrompt, harnesssettings.Draft and settings pane; plantel.Policy renamed to AuthoringPolicy (constants AuthoringAdaptive/AuthoringLegacy)
- doc/project-layout.md lists internal/plantel, internal/planscen and doc/plan-authoring.md; CHANGELOG.md [Unreleased] covers the fixes; doc/plan-authoring.md claims match reality
- make fmt-check lint test green; conventional commits, one logical change each; main clean after merge

## Verification Plan

1. go test ./internal/session/... ./internal/agent/... ./internal/plantel/... ./internal/plangate/... ./internal/tools/plantool/... ./internal/harnesssettings/... ./internal/tui/...
2. grep: no hand-enumerated completed/cancelled/superseded outside Terminal() and switch statements with distinct semantics
3. grep: DraftCreated/ApprovalLatency/MaterialReapproval/PatchRetry/CompletionSuccess/CompletionAbandoned have production call sites outside plantel tests
4. make fmt-check lint test in the worktree before merge to main

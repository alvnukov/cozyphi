---
id: isolate-edit-capability-on-session-switch
title: Retire file edit capabilities when switching sessions
status: done
priority: high
model_level: medium
task_type: bug
parent_id: reliable-model-file-edits
acceptance_criteria:
    - Same-engine session switch cannot use prior session editable anchors.
    - Newly observed anchors in the switched session authorize normal edit/write successor chains.
    - Compaction in the same session preserves valid capabilities; default and explicit built-in tool assemblies are covered.
verification_plan:
    - Reproduce via Engine ReplaceSession with real built-in read/edit tools.
    - Targeted agent/tools tests plus relevant race tests and language-server diagnostics.
created_at: "2026-09-05T09:45:43.901252Z"
updated_at: "2026-09-05T10:14:27.468207Z"
---

## Body

Integration review: Engine.ReplaceSession swaps history and executor metadata but retains tool closures capturing the previous session's editledger. An editable observation from session A can authorize a write after switching to B. Fresh Engine resume creates a new ledger; same-engine /resume does not. Isolate capabilities across history switches without weakening permission gates or mutating caller-owned custom tool state.

**Accepted and integrated (2026-09-05).** Reproduced retained grants after ReplaceSession. Built-in tools now rebuild session-owned capability closures at Engine construction and history switches; ordinary refresh preserves current grants. Tests cover implicit/explicit defaults, reused tool slices across engines, fresh observation/write successors, empty toolsets and unmarked custom handlers. Integrated 59ec6bb; narrow and race tests pass. Final implementation integrated into main at 66d047d. Full make fmt-check passed, followed by scoped formatting of final lint corrections and merged upstream files; final make lint test passed (QUALITY_EXIT=0). Relevant race gate passed (RACE_EXIT=0). Python ruff and mypy --strict passed. Evidence: doc/edit-reliability-evaluation.md and local .mcp-ai-helper/notes/edit-eval-20260905/.

## Acceptance Criteria

- Same-engine session switch cannot use prior session editable anchors.
- Newly observed anchors in the switched session authorize normal edit/write successor chains.
- Compaction in the same session preserves valid capabilities; default and explicit built-in tool assemblies are covered.

## Verification Plan

1. Reproduce via Engine ReplaceSession with real built-in read/edit tools.
2. Targeted agent/tools tests plus relevant race tests and language-server diagnostics.

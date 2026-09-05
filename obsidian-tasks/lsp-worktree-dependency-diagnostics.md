---
id: lsp-worktree-dependency-diagnostics
title: Fix stale dependency diagnostics in worktree modules
status: todo
priority: low
model_level: medium
task_type: bug
tags:
    - lsp
    - harness
acceptance_criteria:
    - Worktree diagnostics reflect edited nested-module dependencies or disclose stale provenance.
verification_plan:
    - Reproduce dependency declaration edits in nested xui module and compare diagnostics with go test.
created_at: "2026-09-05T23:36:19.4203Z"
updated_at: "2026-09-05T23:36:19.4203Z"
---

## Body

During safe-paste-limit in .worktrees/safe-paste-limit, scoped go test ./input (xui) and root chat/composer/editor all passed, but harness lsp diagnostics labeled fresh report undefined input.MaxPasteBytes and xui.PasteRejectedEvent. Both declarations exist in edited xui/input/parser.go and event.go/alias.go. Explicit diagnostics of event.go/alias.go report none; root chat still reports undefined constant. Investigate worktree/nested-module dependency synchronization and provenance.

## Acceptance Criteria

- Worktree diagnostics reflect edited nested-module dependencies or disclose stale provenance.

## Verification Plan

1. Reproduce dependency declaration edits in nested xui module and compare diagnostics with go test.

---
id: code-mode-redesign
title: Design and evaluate a programmatic tool interface for CozyPhi
status: todo
priority: high
task_type: feature
branch: feature/code-mode-redesign
worktree_path: .worktrees/code-mode-redesign
tags:
  - tools
  - mcp
  - evaluation
acceptance_criteria:
  - Document the public contracts and measured trade-offs of comparable agent harnesses.
  - Map every existing tool family and execution posture to the proposed interface.
  - Specify a harness-wide library of discoverable, editable, versioned functions with executable unit tests.
  - Require immutable package versions, per-run dependency pins, isolated concurrent drafts, stale-publisher detection and reproducible rollback across projects.
  - Deliver a callable matching export or an actionable extension candidate before the main model's first response.
  - Compare automatic local selection and a cheap advisory model by correct reuse, unnecessary duplicates, total cost and rounds to useful work.
  - Keep hints optional and preserve immediate access to every session-authorized operation, including choices omitted by the advisor.
  - Exercise transient programs, reusable functions and tests of generated project code as distinct outcomes.
  - Specify a migration with independently verifiable milestones and a paired evaluation against the current engine.
  - Select runtime and rollout defaults using recorded results on the target models.
verification_plan:
  - Check design assumptions against the current registry, executor, MCP transport and session code.
  - Run deterministic equivalence and recovery tests before any model trials.
  - Run matched direct and programmatic tasks with exact outcome checks before promoting a default.
  - Verify that a fresh session in a second repository can find, test, invoke and revise the same function.
  - Compare manual discovery, local selection, conditional advice and always-on advice on matched reuse, extension and genuine-miss tasks, including a stale or unavailable advisor.
  - Inject a plausible wrong hint and verify an unlisted alternative can run on the first response through the ordinary dispatcher with identical permissions.
created_at: "2026-11-22T09:41:17Z"
updated_at: "2026-11-22T10:10:08Z"
---

## Body

The requested deliverable is a proposal for replacing the model-facing tool interface.
The design and acceptance criteria live in `specs/code-mode-redesign.md` on the task branch.
Implementation and benchmark results are future milestones.

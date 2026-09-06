---
id: subscription-routing-spec
title: Specify subscription-aware tier routing and implementation tickets
status: done
priority: high
model_level: high
task_type: docs
tags:
    - planning
    - routing
branch: docs/subscription-routing-spec
worktree_path: .worktrees/subscription-routing-spec
acceptance_criteria:
    - Publish an English specification and domain vocabulary for the agreed opt-in subscription-aware tier routing contract.
    - Create an epic and dependency-linked implementation tickets with acceptance criteria and verification plans, plus a separate linked user root-override ticket.
    - Review and commit only owned documentation and task ledger files; make no runtime changes or live provider calls.
verification_plan:
    - Check the specification against every confirmed interview decision and source-evidence limitation.
    - Verify ticket dependency graph, acceptance criteria, verification plans and document links.
    - Review the owned diff and confirm no executable files, credentials or unrelated ledger notes are included.
created_at: "2026-09-06T11:01:42.532596Z"
updated_at: "2026-09-06T11:19:12.945871Z"
---

## Body

Document the completed user interview on automatic model/native-effort selection by user-assigned quality tiers, account-wide subscription budgeting, adaptive reserve and useful surplus spending. This is planning only; implementation remains future work. Documents belong in a dedicated worktree; registry mutations stay on main through task.

**Started (2026-09-06).** Confirmed interview is complete. Preparing documentation only; no implementation, credential reads, live spending or current-model changes.

**Note (2026-09-06).** Created English specification, approved nine-slice breakdown, historical source-evidence record and routing glossary in the dedicated docs/subscription-routing-spec worktree. Created the epic, nine dependency-blocked implementation notes and separate user-root-tool-override note on main. Structural checks passed for four documents, twelve owned notes, all local links, exact blocker lists, acyclic graph and S01–S20. Independent read-only review found no must-fix issues; added its suggested delayed-launch plus grant-revocation scenario. No implementation, credentials, provider calls or Go tests.

**Done (2026-09-06).** Documentation commit df88428 merged into main as 5ea2cb2: English specification, approved nine-slice dependency/coverage map, source-evidence limitations and CONTEXT glossary. Published subscription-aware-routing epic, nine blocked implementation slices and independent user-root-tool-override task; all have acceptance and verification. Local-link/graph/scenario/whitespace checks passed; independent review found no must-fix issues and its grant-revocation scenario was added. Planning only: no code implementation, credential access, live provider calls or model change. Epic and implementation tasks remain open; committing the owned ledger separately.

## Acceptance Criteria

- Publish an English specification and domain vocabulary for the agreed opt-in subscription-aware tier routing contract.
- Create an epic and dependency-linked implementation tickets with acceptance criteria and verification plans, plus a separate linked user root-override ticket.
- Review and commit only owned documentation and task ledger files; make no runtime changes or live provider calls.

## Verification Plan

1. Check the specification against every confirmed interview decision and source-evidence limitation.
2. Verify ticket dependency graph, acceptance criteria, verification plans and document links.
3. Review the owned diff and confirm no executable files, credentials or unrelated ledger notes are included.

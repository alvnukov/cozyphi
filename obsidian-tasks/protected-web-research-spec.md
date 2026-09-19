---
id: protected-web-research-spec
title: Specify protected asynchronous web research
status: done
priority: high
model_level: high
task_type: docs
parent_id: web-tools
tags:
    - web
    - security
    - spec
branch: docs/protected-web-research-spec
worktree_path: .worktrees/protected-web-research-spec
acceptance_criteria:
    - Standalone English specification captures the 43 confirmed web design decisions and approved behavioral testing boundaries.
    - Security, routing and existing web contracts are reconciled explicitly; requirements are distinguished from implemented behavior and unmeasured feasibility.
    - The web-tools epic links to the specification and no longer describes live smoke testing as the only remaining work.
    - Only specification and owned registry notes change; no implementation plan, runtime code or configuration changes.
verification_plan:
    - Check the specification against all 43 confirmed discussion decisions and the to-spec template.
    - Check consistency with current web documentation and shared harness-security and subscription-routing specifications.
    - Validate local links and git diff --check only; no Go gates for a documentation-only change.
    - Commit only the specification and its registry notes on the task branch.
created_at: "2026-09-19T16:04:37.972385Z"
updated_at: "2026-09-19T20:50:04.991412Z"
---

## Body

Synthesize the confirmed interactive design discussion into a standalone specification for protected asynchronous web research. Cover the explicitly configured web model, parallel safety/extraction with decoys, final screening, public multi-format acquisition, whole-harness enforcement, durable provenance, incident review and hostname blocking, bounded background execution, and behavioral acceptance. The user confirmed the proposed public testing boundaries. This task publishes requirements only; it does not authorize implementation, live model tests, or an executable plan.

**Started (2026-09-19).** User requested to-spec after confirming 43 interview decisions and the public behavioral testing boundaries. Documentation only; no executable plan or runtime implementation.

**Note (2026-09-19).** Prepared [Protected asynchronous web research](../specs/protected-web-research.md): 62 user stories, 16 decision sections, 32 behavioral scenarios and complete Q1–Q43 traceability. Updated web-tools acceptance and scope; preserved historical implementation notes. Independent read-only spec review confirmed all interview decisions and template sections, with clarifications incorporated for successful-recheck-only user unblock, post-pass action grants, third-party resource revocation and the explicitly selected OS-backed key handling. Verification passed: seven template sections, sequential stories/decisions/scenarios, all 43 decisions, local link targets, whitespace and git diff --check. Documentation only; no Go gates, live model calls or runtime changes. Work stays in progress pending authorized PR publication/review/merge; no remote publication or merge has been requested.

**Done (2026-09-19).** Спецификация specs/protected-web-research.md (62 истории, 16 разделов решений, 32 сценария, трейс Q1–Q43) смержена в main (коммит aed5af09 дошёл через PR #41).

## Acceptance Criteria

- Standalone English specification captures the 43 confirmed web design decisions and approved behavioral testing boundaries.
- Security, routing and existing web contracts are reconciled explicitly; requirements are distinguished from implemented behavior and unmeasured feasibility.
- The web-tools epic links to the specification and no longer describes live smoke testing as the only remaining work.
- Only specification and owned registry notes change; no implementation plan, runtime code or configuration changes.

## Verification Plan

1. Check the specification against all 43 confirmed discussion decisions and the to-spec template.
2. Check consistency with current web documentation and shared harness-security and subscription-routing specifications.
3. Validate local links and git diff --check only; no Go gates for a documentation-only change.
4. Commit only the specification and its registry notes on the task branch.

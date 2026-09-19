---
id: web-research-30-incident-viewer
title: 30 — Inspect incidents in a safe human-only view
status: blocked
priority: medium
model_level: low
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Incident details are reachable only through an explicit human UI action, never a model tool/context/log route.
    - Terminal controls and active content are neutralized; metadata/detail sizes are bounded and sensitive details protected.
    - Opening the view does not execute tools, open a URL, recheck or unblock; unavailable protected detail storage is reported.
verification_plan:
    - Render escape/control, forged-approval, long-URL and synthetic-secret fixtures using the actual viewer boundary.
    - Capture parent context/log outputs and prove no incident payload delivery or tool execution.
    - Test external open requires a separate human action; run scoped UI/controller tests.
created_at: "2026-09-19T19:18:44.955052Z"
updated_at: "2026-09-19T19:18:44.955052Z"
---

## Body

**What to build:** A user-only incident view consuming the existing incident state, not a model-readable debug page.

**Blocked by:** [20](web-research-20-safe-progress.md), [26](web-research-26-incident-kinds.md).

**Contract:** [Spec](../specs/protected-web-research.md), D10/D11, Q35/Q36/Q43, T14/T22/T24. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Render safe hostname/URL, source/version, stage, model/policy identity, time and observed event using existing host UI. Details remain bounded, secret-filtered and temporary unless protected storage already exists. Reuse existing safe text/control neutralization. External opening must be a separate human event, not click-like text interpreted from evidence. The widget does not decide block policy; recheck/unblock behavior is 31.

**Do not change:** No tool-call argument replay, payload in routine transcript, new storage backend or model-accessible incident read action.

**Proof required:** Hostile terminal/link/approval fixtures render inertly; parent request captures, tool catalog, logs and normal file routes do not expose detailed material. Show view/open intent as separate user events.

**Stop condition:** If no safe human-only data route exists, block detail display rather than reuse ordinary model transcript storage.

## Acceptance Criteria

- Incident details are reachable only through an explicit human UI action, never a model tool/context/log route.
- Terminal controls and active content are neutralized; metadata/detail sizes are bounded and sensitive details protected.
- Opening the view does not execute tools, open a URL, recheck or unblock; unavailable protected detail storage is reported.

## Verification Plan

1. Render escape/control, forged-approval, long-URL and synthetic-secret fixtures using the actual viewer boundary.
2. Capture parent context/log outputs and prove no incident payload delivery or tool execution.
3. Test external open requires a separate human action; run scoped UI/controller tests.

---
id: web-research-20-safe-progress
title: 20 — Show safe research progress without model-content leakage
status: blocked
priority: medium
model_level: low
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - User progress shows only bounded host-owned stages/counts/reasons, without source text, model drafts or unchecked titles.
    - Progress and errors cannot spoof approvals, links/actions or terminal controls.
    - Progress does not inject content into main-model context or autonomously deliver partial answers.
verification_plan:
    - Render fixed host-owned states with malicious metadata/control-sequence fixtures.
    - Assert transcript/model-context separation and absence of source/draft sentinels.
    - Run only touched rendering/controller tests and capture representative output.
created_at: "2026-09-19T19:15:57.455544Z"
updated_at: "2026-09-19T19:15:57.455544Z"
---

## Body

**What to build:** Safe visible progress for a running research job using existing job/TUI rendering seams.

**Blocked by:** [06](web-research-06-single-source-tracer.md).

**Contract:** [Spec](../specs/protected-web-research.md), D3/D10/D12, Q17/Q24/Q36, T14/T26. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Map host-owned job stages, counts and bounded reason codes into the existing status/notice interface. Widgets only render; they do not decide admission. Normalize control characters and bound user-visible metadata. Never use provider free text or page title as a stage label. Keep progress separate from model messages and checked-result retrieval.

**Do not change:** No automatic partial-result stream, new scheduler, invented completion percentage or payload-bearing routine logs.

**Proof required:** UI/render snapshots for running, denied, cancelled and completed jobs plus hostile title/error/tool-argument sentinels. Capture parent model messages and prove progress adds no source/draft content.

**Stop condition:** If the existing notice route automatically feeds text to the model, separate safe metadata delivery before using it.

## Acceptance Criteria

- User progress shows only bounded host-owned stages/counts/reasons, without source text, model drafts or unchecked titles.
- Progress and errors cannot spoof approvals, links/actions or terminal controls.
- Progress does not inject content into main-model context or autonomously deliver partial answers.

## Verification Plan

1. Render fixed host-owned states with malicious metadata/control-sequence fixtures.
2. Assert transcript/model-context separation and absence of source/draft sentinels.
3. Run only touched rendering/controller tests and capture representative output.

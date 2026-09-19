---
id: web-research-26-incident-kinds
title: 26 — Classify quarantine incidents without inventing site attribution
status: blocked
priority: high
model_level: low
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Source-attributed decoy/unknown call, semantic suspicion, technical error and unattributed multi-source decoy have distinct bounded outcomes.
    - Only observed source-attributed quarantine calls request hostname blocking; suspicion alone quarantines snapshot.
    - No incident claims malicious site intent or forwards hostile payload to routine outputs.
verification_plan:
    - Run the fixed event classification table and one existing single-source public incident; no multi-source pipeline implementation in this task.
    - Assert only source-attributed tool-call observations request hostname blocking; record 42 as owner of actual multi-source integration regression.
    - Check hostile arguments/error strings and synthetic secrets do not appear in routine status/log output.
created_at: "2026-09-19T19:17:18.920947Z"
updated_at: "2026-09-19T19:32:44.094192Z"
---

## Body

**What to build:** An explicit incident classification table consumed by future block/review behavior.

**Blocked by:** [07](web-research-07-invalid-responses.md).

**Contract:** [Spec](../specs/protected-web-research.md), D9/D10, Q6/Q27/Q28/Q36, T04/T05/T18. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Map observed quarantine events into host-owned kinds and source/snapshot, answer/research or technical scopes. Retain bounded identity, stage/configuration/policy/time. An unattributed multi-source final call cannot mark all hosts. This slice tests a fixed event table through the incident boundary; it does not require or construct the not-yet-delivered multi-source pipeline. Ticket 42, blocked by 17 and 26, owns additional real multi-source final-stage integration regression. Keep detailed payload protected/temporary and separate from safe metadata.

**Do not change:** No multi-source orchestration, detector threshold invention, repeated rechecks, broad-domain blocking or accusation text.

**Proof required:** Each fixed source and multi-source-labelled event produces the expected safe outcome and block-request scope; the already available single-source public pipeline exercises one real incident. Clearly label synthetic event tests versus pipeline integration. Network timeout/refusal never proves malicious intent; unknown tool names are attempted calls.

**Stop condition:** Missing attribution stays unknown and restricts the result; never infer a site from the last citation.

## Acceptance Criteria

- Source-attributed decoy/unknown call, semantic suspicion, technical error and unattributed multi-source decoy have distinct bounded outcomes.
- Only observed source-attributed quarantine calls request hostname blocking; suspicion alone quarantines snapshot.
- No incident claims malicious site intent or forwards hostile payload to routine outputs.

## Verification Plan

1. Run the fixed event classification table and one existing single-source public incident; no multi-source pipeline implementation in this task.
2. Assert only source-attributed tool-call observations request hostname blocking; record 42 as owner of actual multi-source integration regression.
3. Check hostile arguments/error strings and synthetic secrets do not appear in routine status/log output.

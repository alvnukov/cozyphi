---
id: web-research-12-question-search
title: 12 — Research a question without exposing unchecked search results
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - The main model submits a question rather than orchestrating search/fetch/read calls.
    - Titles, snippets and generated follow-up proposals cannot reach the parent or acquire authority unchecked.
    - Each actual search/acquisition stays within current research consent; insufficient evidence is explicit.
verification_plan:
    - Drive one question through controlled search, selected fetch and checked answer.
    - Inject attacks in titles/snippets/error text and proposed next queries; assert screened/withheld content and recipient revalidation.
    - Compare authorized and denied outbound payload captures; run scoped web/engine tests.
created_at: "2026-09-19T19:13:21.108237Z"
updated_at: "2026-09-19T19:13:21.108237Z"
---

## Body

**What to build:** A bounded search-to-answer path built on the one-static-source workflow.

**Blocked by:** [11](web-research-11-static-answer.md).

**Contract:** [Spec](../specs/protected-web-research.md), D3/D5/D8/D14, Q2/Q7/Q17/Q31/Q41, T14/T15/T16. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Use existing search adapters after checking their service contract. Treat result titles/snippets as source material requiring admission, not safe metadata. The quarantined model may propose next work only as bounded data; the harness validates source choice, grant, recipient and limits before execution. No real search executor exists in a call reading unchecked content. Initially select one answer source; multi-source combination belongs to 17.

**Do not change:** No general agent/browser tool loop, model-authored DAG, parent-history export or automatic out-of-scope link following.

**Proof required:** Search request and result fixtures with injected title/snippet and rewritten confidential follow-up; parent receives no unchecked snippets and denied follow-up sinks receive zero traffic. A benign query produces a cited answer via the same path.

**Stop condition:** Ambiguous confidentiality pauses through 05 rather than model-based anonymization.

## Acceptance Criteria

- The main model submits a question rather than orchestrating search/fetch/read calls.
- Titles, snippets and generated follow-up proposals cannot reach the parent or acquire authority unchecked.
- Each actual search/acquisition stays within current research consent; insufficient evidence is explicit.

## Verification Plan

1. Drive one question through controlled search, selected fetch and checked answer.
2. Inject attacks in titles/snippets/error text and proposed next queries; assert screened/withheld content and recipient revalidation.
3. Compare authorized and denied outbound payload captures; run scoped web/engine tests.

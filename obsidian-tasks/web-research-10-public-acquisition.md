---
id: web-research-10-public-acquisition
title: 10 — Enforce public-only acquisition at redirects and actual dial targets
status: blocked
priority: high
model_level: medium
task_type: feature
parent_id: web-tools
tags:
    - protected-web-research
acceptance_criteria:
    - Private, loopback, link-local, credential-bearing, disallowed-scheme and rebinding targets are refused at actual dispatch.
    - Redirects and alternate host representations obey the same public-only policy; legacy allowlists cannot override it.
    - Source/decompressed bytes, redirects and time are bounded; no ambient user credentials are sent.
verification_plan:
    - Exercise controlled DNS/dial/redirect fixtures covering IPv4/IPv6, rebinding, userinfo, private ranges and legacy allowlists.
    - Check forbidden-sink counters, decompression/size/time bounds and absence of cookies/credentials.
    - Record authoritative dependency evidence and scoped transport integration tests; no live network attacks.
created_at: "2026-09-19T19:13:20.936969Z"
updated_at: "2026-09-19T19:13:20.936969Z"
---

## Body

**What to build:** Public-only static acquisition through the existing web network boundary, with measurable refusal at the destination rather than URL-string checks alone.

**Blocked by:** [05](web-research-05-research-consent.md).

**Contract:** [Spec](../specs/protected-web-research.md), D4/D8, Q3/Q7, T11/T12/T15/T16. Follow the [delivery rules](../specs/protected-web-research-tickets.md).

**Work:** Inspect the actual cozy-tools acquisition/redirect/dial policy and its authoritative implementation before changes. Reuse or minimally extend it, not a competing HTTP stack. Resolve/canonicalize and validate actual dial addresses; revalidate each redirect and effective outbound request. Reject userinfo/private endpoints despite old allowed_hosts. Limit compressed/decompressed bodies, type, redirects and duration. Unit fixtures may use a controlled transport without adding a production loopback exception.

**Do not change:** No browser implementation, guessed protocol behavior, real internet probing, credential inheritance or blanket network grants.

**Proof required:** Controlled forbidden sinks receive zero requests for rebinding, redirect and alternate-IP cases; a permitted public-target fixture succeeds with expected bounded bytes. Capture grant checks at final dispatch.

**Stop condition:** Dependency changes need a real published dependency/owner workflow, never a committed local replace. This ticket proves acquisition policy, not whole-harness isolation.

## Acceptance Criteria

- Private, loopback, link-local, credential-bearing, disallowed-scheme and rebinding targets are refused at actual dispatch.
- Redirects and alternate host representations obey the same public-only policy; legacy allowlists cannot override it.
- Source/decompressed bytes, redirects and time are bounded; no ambient user credentials are sent.

## Verification Plan

1. Exercise controlled DNS/dial/redirect fixtures covering IPv4/IPv6, rebinding, userinfo, private ranges and legacy allowlists.
2. Check forbidden-sink counters, decompression/size/time bounds and absence of cookies/credentials.
3. Record authoritative dependency evidence and scoped transport integration tests; no live network attacks.

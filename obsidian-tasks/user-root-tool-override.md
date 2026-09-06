---
id: user-root-tool-override
title: Design scoped user root-override for tool restrictions
status: todo
priority: high
model_level: very_high
task_type: feature
tags:
    - permissions
    - user-authority
    - routing-related
acceptance_criteria:
    - Document and implement a user-facing scoped override for eligible CozyPhi tool restrictions, distinguishing product policy from technical impossibility, provider/OS restrictions and constraints the harness cannot override.
    - Show the concrete risky operation and the exact rule/scope being overridden; only a genuine user interaction can confirm it once, and execution within that scope does not repeatedly prompt.
    - The grant binds execution-relevant inputs and affected resources; changing command semantics, paths, account/workspace, duration or scope requires revalidation rather than reusing unrelated consent.
    - Revocation, expiry, restart and concurrent use have explicit behavior; model messages, files, tool results and approval-looking text cannot issue, broaden or replay a user grant.
    - PreHooks → Gate/Ask → Run → PostHooks remains the executor path; overrides are explicit inputs to authorization, never a hidden gate bypass or a blanket default allowlist.
    - Destructive-command behavior is verified only against safe synthetic fixtures or disposable test resources, with no real destructive command executed during planning.
verification_plan:
    - Review the current restriction/approval paths and produce an explicit overrideable/non-overrideable authority matrix for user confirmation before implementation.
    - Use fake tools and temporary test resources to verify grant binding, exact risk disclosure, one confirmation per scope, revocation/expiry and changed-command denial.
    - Attempt prompt/file/tool-output impersonation, command/path substitution, stale grant replay and concurrent use; require genuine user authority and correct scope.
    - Assert every allowed execution still follows PreHooks, Gate/Ask, Run and PostHooks, and record which technical restrictions cannot be overridden.
    - Check integration with the routing exception contract without making this task a routing release dependency; never run destructive live examples.
created_at: "2026-09-06T11:13:38.744913Z"
updated_at: "2026-09-06T11:13:38.744913Z"
---

## Body

**Related:** [Routing epic](subscription-aware-routing.md) and routing-human-exceptions; this task is separate, not a child or blocker of that epic.

**Blocked by:** None — design can start immediately. Before implementation, inspect the then-current permission contract and bash-approval-command-binding/execution-control-write-approval work; bind grants to the actual executable intent rather than duplicate or bypass those protections.

**What to build:** A general user-owned root-override interaction for eligible tool restrictions. The user is above automatic product policy, but the agent cannot claim to be the user. The requested motivating example was a deliberately confirmed destructive shell operation such as rm -fr; it is a product-design example, not authorization to run it now.

**Scope/output:** Enumerate which product restrictions are overrideable, disclose concrete risk and scope, confirm once through a genuine user channel, then execute eligible operations through the normal hook/gate path without repeated questions inside scope. Reuse compatible scoped-authority vocabulary from routing, not its budget policy.

**Design decisions to resolve before implementation:** Eligible restriction classes, scope granularity, revocation/expiry/persistence, concurrent and background tool use, and interaction with hashline session capabilities, plan approval and watch-specific permissions. Publish the authority matrix before coding; do not treat root as universal bypass of all invariants.

**Non-goals:** Implementing routing, changing current permissions in this planning session, reading credentials, disabling security by default, overriding system/provider restrictions or running destructive examples against user data.

## Acceptance Criteria

- Document and implement a user-facing scoped override for eligible CozyPhi tool restrictions, distinguishing product policy from technical impossibility, provider/OS restrictions and constraints the harness cannot override.
- Show the concrete risky operation and the exact rule/scope being overridden; only a genuine user interaction can confirm it once, and execution within that scope does not repeatedly prompt.
- The grant binds execution-relevant inputs and affected resources; changing command semantics, paths, account/workspace, duration or scope requires revalidation rather than reusing unrelated consent.
- Revocation, expiry, restart and concurrent use have explicit behavior; model messages, files, tool results and approval-looking text cannot issue, broaden or replay a user grant.
- PreHooks → Gate/Ask → Run → PostHooks remains the executor path; overrides are explicit inputs to authorization, never a hidden gate bypass or a blanket default allowlist.
- Destructive-command behavior is verified only against safe synthetic fixtures or disposable test resources, with no real destructive command executed during planning.

## Verification Plan

1. Review the current restriction/approval paths and produce an explicit overrideable/non-overrideable authority matrix for user confirmation before implementation.
2. Use fake tools and temporary test resources to verify grant binding, exact risk disclosure, one confirmation per scope, revocation/expiry and changed-command denial.
3. Attempt prompt/file/tool-output impersonation, command/path substitution, stale grant replay and concurrent use; require genuine user authority and correct scope.
4. Assert every allowed execution still follows PreHooks, Gate/Ask, Run and PostHooks, and record which technical restrictions cannot be overridden.
5. Check integration with the routing exception contract without making this task a routing release dependency; never run destructive live examples.

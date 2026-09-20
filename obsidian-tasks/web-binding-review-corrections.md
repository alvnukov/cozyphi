---
id: web-binding-review-corrections
title: Fix secret exposure and incomplete web model binding identity
status: todo
priority: high
model_level: high
task_type: bug
parent_id: web-tools
tags:
    - protected-web-research
    - review
acceptance_criteria:
    - Model-facing identity and not-ready results never disclose credentials embedded in endpoint userinfo, path or query.
    - The binding distinguishes actual account/connection identity without including authentication tokens, or explicitly reports the missing identity contract as not-ready.
    - Changes to all effective request parameters, including MaxOutputTokens, invalidate the binding key.
    - Removal, connection or route change is reflected at web admission without rebuilding the session.
verification_plan:
    - Add a boundary regression for endpoint credentials in not-ready output; assert zero acquisition/model calls and no secret in Content/Output/Detail.
    - Test distinct account connections and effective request options without equating token rotation to account change.
    - Test live missing-to-connected and changed/removed binding admission scenarios.
    - Run only affected package tests and scoped gates during implementation.
created_at: "2026-09-20T08:06:10.513899Z"
updated_at: "2026-09-20T08:06:10.513899Z"
---

## Body

Review of web-research-01/02/03 at ec8a7b0b (PRs #42/#43/#45) found four defects in ticket 03. **P1:** internal/project/web_binding.go:82-86,144-147 copies raw BaseURL into Identity/NotReadyReason; internal/tools/webtool/web.go:265-269 sends it into Content/Output/Detail. Endpoint credentials can leak even while web stays not-ready. **P2:** fingerprint at web_binding.go:100-120 lacks account identity; oauthAuthenticator resolves the current credential by provider ID (internal/provider/oauth.go:634-660), so a different account is not a different binding. Task03 explicitly requires documenting a blocker if the identity contract is unavailable. **P2:** fingerprint omits MaxOutputTokens although internal/llm/openai/client.go:135 uses it in requests. **P2:** web.go:46-50 captures binding once and :180-183 copies a fixed refusal; provider connection changes do not update it. References: original task03:33-39 and spec D2:153-158. Production Ready remains false; this review did not find a web-content release bypass. Findings are static source/LSP evidence, not a runtime reproduction.

## Acceptance Criteria

- Model-facing identity and not-ready results never disclose credentials embedded in endpoint userinfo, path or query.
- The binding distinguishes actual account/connection identity without including authentication tokens, or explicitly reports the missing identity contract as not-ready.
- Changes to all effective request parameters, including MaxOutputTokens, invalidate the binding key.
- Removal, connection or route change is reflected at web admission without rebuilding the session.

## Verification Plan

1. Add a boundary regression for endpoint credentials in not-ready output; assert zero acquisition/model calls and no secret in Content/Output/Detail.
2. Test distinct account connections and effective request options without equating token rotation to account change.
3. Test live missing-to-connected and changed/removed binding admission scenarios.
4. Run only affected package tests and scoped gates during implementation.

# Codex usage contract

CozyPhi's ChatGPT OAuth adapter follows the official `openai/codex` source at
`531f3836a1e38ea61eaaba3dccda6711eb6c0dca`. Tests follow that contract; live
verification is recorded separately below.

## Pinned evidence

- [Backend client](https://github.com/openai/codex/blob/531f3836a1e38ea61eaaba3dccda6711eb6c0dca/codex-rs/backend-client/src/client.rs):
  `PathStyle::ChatGptApi`, `get_token_usage_profile`, `token_usage_profile_url`,
  `map_rate_limit_window`, and additional-limit mapping.
- [Usage/reset requests](https://github.com/openai/codex/blob/531f3836a1e38ea61eaaba3dccda6711eb6c0dca/codex-rs/backend-client/src/client/rate_limit_resets.rs):
  GET `/wham/usage`; POST `/wham/rate-limit-reset-credits/consume` with
  `redeem_request_id` and optional `credit_id`. Passive readers do not opt into Luna Reserve.
- [Wire types](https://github.com/openai/codex/blob/531f3836a1e38ea61eaaba3dccda6711eb6c0dca/codex-rs/backend-client/src/types.rs):
  `TokenUsageProfile`, `TokenUsageProfileStats`, `TokenUsageProfileDailyBucket`,
  and `RateLimitResetCreditsSummary`.
- [Upstream fixtures](https://github.com/openai/codex/blob/531f3836a1e38ea61eaaba3dccda6711eb6c0dca/codex-rs/backend-client/src/client/rate_limit_resets_tests.rs):
  WHAM paths, nested windows, Unix-second resets and optional reset summaries.
- [App-server documentation](https://github.com/openai/codex/blob/531f3836a1e38ea61eaaba3dccda6711eb6c0dca/codex-rs/app-server/README.md):
  account rate limits, separate token activity, and reset consumption are distinct
  operations. Its camelCase RPC responses are **not** backend HTTP wire models.
- [Generated usage payload](https://github.com/openai/codex/blob/531f3836a1e38ea61eaaba3dccda6711eb6c0dca/codex-rs/codex-backend-openapi-models/src/models/rate_limit_status_payload.rs),
  [window](https://github.com/openai/codex/blob/531f3836a1e38ea61eaaba3dccda6711eb6c0dca/codex-rs/codex-backend-openapi-models/src/models/rate_limit_window_snapshot.rs), and
  [additional limits](https://github.com/openai/codex/blob/531f3836a1e38ea61eaaba3dccda6711eb6c0dca/codex-rs/codex-backend-openapi-models/src/models/additional_rate_limit_details.rs):
  HTTP field names, optional nested objects and integer window seconds.

## Read-only mapping

The stored `/backend-api/codex` credential base becomes the same origin's
`/backend-api/wham/usage`, with a separate GET to
`/backend-api/wham/profiles/me`. Other base paths are rejected rather than guessed.
Only these two exact GET targets may receive OAuth headers; redirects, queries,
encoded paths, origin changes and mutation paths are rejected. Existing OAuth
refresh, bearer/account and residency headers are reused. Responses are bounded
at 1 MiB, cancellation is honored, and errors omit bodies and transport details.

Usage fields are `plan_type`, `rate_limit.primary_window` / `secondary_window`,
and `additional_rate_limits[].rate_limit` with `limit_name` (or
`metered_feature` as a fallback label). Windows expose `used_percent`,
`limit_window_seconds`, and `reset_at` in Unix **seconds**. No absolute token
budget is inferred from a percentage. Missing/null windows or percentages do
not become observed zero usage.

Profile counts from `stats.lifetime_tokens` and
`stats.daily_usage_buckets[{start_date,tokens}]` remain decoded for compatibility,
but are deliberately **not displayed** in `/usage` or the dashboard. They are not
needed to decide whether to reset a limit. The compact subscription section shows
plan, observed limits/remaining percentages, reset times and reset credits; the
separate Session section shows only the current CozyPhi session's tokens.
Optional profile failures retain good limits. Optional
`rate_limit_reset_credits.available_count` comes from the usage response itself;
a missing or malformed summary does not invalidate limits or become zero.

## Explicit reset

Only the standalone `/usage` pane offers **Reset limit** (`x`). It requires a
second, explicit confirmation to spend one reset credit (`y` or Confirm).
`n`, Esc or Cancel withdraws confirmation without sending a reset. The dashboard
remains a compact read-only summary. A positive observed credit count enables the
action; actual eligibility is decided by the server, not inferred from a usage
percentage.

The mutation uses a separate exact POST target:
`/backend-api/wham/rate-limit-reset-credits/consume`. Its JSON body carries a fresh
`redeem_request_id`; `credit_id` is omitted so the server selects the credit.
Known response codes are `reset`, `nothing_to_reset`, `no_credit`, and
`already_redeemed`, with `windows_reset` on a reset result. Unknown/malformed
responses do not count as success.

The confirmation target is opaque, single-use and bound to the connected OAuth
account. Account/model changes withdraw stale confirmation; the provider checks
account binding again after any token refresh and never silently redirects the
operation to a newly selected account. Duplicate confirmations and concurrent
attempts are blocked. Redirects are rejected, bodies are bounded, cancellation
is honored, and errors omit credentials, account IDs, response bodies and raw
transport details.

After completion, the shell requests fresh quotas and ignores pre-reset reads.
A reset success and a failed subsequent refresh remain separate outcomes.
A timeout, disconnect or unrecognized response after possible transmission means
the **outcome is unknown**: reconcile with a read; never retry automatically.
No reset listing, Luna Reserve opt-in or other account mutation is added.

## Verification limits

Regression tests use local HTTP servers and synthetic credentials. **No live
credit-consuming requests are run during development or verification.** The
reset implementation is source/fixture-verified, not claimed live-verified.
The original upstream-contract test failed with HTTP 404 before the route correction.

On 2026-09-05, a separate opt-in probe ran the corrected adapter against the
connected ChatGPT account: both GET endpoints returned HTTP 200; the adapter
decoded a rate-limit window, profile token scopes and the reset summary. The
probe allowed only those two HTTPS GET targets, rejected redirects, refused
expired credentials rather than refreshing, and logged only HTTP status and
structural counts. No credentials, account identifiers, response bodies or
usage values were logged or saved. No account mutations were performed.

That proves read-only compatibility for the tested account at that time, not
availability for every subscription. Other plans, missing optional data and
failures are covered by contract fixtures, not additional live-account claims.
`code_review_rate_limit` is not mapped: it is absent from the pinned generated
payload and client mapping. Unknown fields are ignored.

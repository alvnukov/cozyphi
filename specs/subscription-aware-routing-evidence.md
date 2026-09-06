# Subscription-aware routing: evidence and verification limits

This is a **source-derived design record**, not a runtime trace or a provider
availability promise. Inspected baseline: `40839909096497ea4f3fa0bd7154bc4285d897e0`
(`main`), 2026-09-06. Read-only investigation used source and key gopls references;
no credentials, provider requests, inference calls or runtime tests were used.
The parent confirmed selection and quota structures against the same baseline.

The implementation contract is [the specification](subscription-aware-routing.md).
Paths below are historical evidence locators, not prescribed implementation sites.

## Current behavior and integration risks

| Evidence locator at baseline | Observation | Consequence for planning |
|---|---|---|
| `internal/llm/types.go:27–66,96–134` | Native effort enum and model-supported choices already exist | Tier must be distinct from native effort despite field reuse |
| `internal/session/plan.go:368–398`; `internal/agent/engine_plan_models.go:11–95` | Step pin, then per-type model, then session; independent native effort overlay; validation errors | Version interpretation and preserve human pins; do not reinterpret old artifacts |
| `internal/agent/engine_plan_actions.go:136–156,189–243`; `engine_plan_active_model.go:10–38` | Validate before start actions; approved switch after actions; multiple entry paths | Cover start, resume, settle, restore and completion, not only normal step advance |
| `internal/tools/agenttool/agent.go:117–120,214–245`; `internal/tools/plantool/effort.go:12–53` | Model-facing concrete model selection is forbidden, effort allowed | Tier request is not authority to rewrite user profiles/pins |
| `internal/agent/engine_runner.go:182–226` | Shared child preparation; stale role pin currently falls back to inheritance | New mode must make stale pins actionable rather than silently substitute |
| `internal/agent/engine.go:433–460,472–532,960–985`; `model_selection.go:11–54` | Immutable inference/tool-round snapshots; delayed child captures originating settings | Keep selection, metadata and delegated intent consistent across switches |
| `internal/llm/options.go:69–90`; `responses/client.go:229–241` | Effective options can outrank config; Responses sends reasoning effort | Validate actual effective wire behavior, not a picker label |
| `internal/llm/anthropic/client.go:214–225` | Current adapter does not send effort/thinking settings | An implementation limitation, not proof that the external provider lacks support |
| `internal/provider/manager.go:160–175,518–524`; `codex_models.go:42–48` | Broad fixed OpenAI effort list; discovery does not read effort capabilities | Existing model-specific effort task is a prerequisite |
| `internal/tui/controller/controller.go:1271–1325`; `cmd/bootstrap.go:165–207` | TUI/headless startup effort restoration differs | Test both paths and artifact-local semantics |
| `internal/agent/engine.go:985–1005`; `internal/llm/apierror.go:63–97`; `internal/runerror/runerror.go:49–57` | No quota reroute today; 429/529 grouped; no structured reset/Retry-After in status error | Classify unavailability before retry; overload is not depleted allowance |
| `internal/provider/quota.go:57–77,111–172,256–280` | OpenAI OAuth adapter reports primary/secondary/additional percentage windows and reset | No absolute token budget from percentage; API-key auth is unsupported by this quota adapter |
| `internal/provider/quota.go:345–395,457–522` | Z.AI token/credit decoding and fallback endpoint exist | Verify units, presence, reset and regional semantics; missing is not zero |
| `internal/provider/quota_reset.go:14–61` | Separately confirmed account-bound manual reset capability | Not adaptive reserve, not permission for automatic reset spending |
| `internal/tui/controller/usage.go:85–139` | Quota fetch currently used for UI; production reference checked with gopls | New admission/coordination behavior is needed, not merely a UI change |
| `internal/tui/controller/usage.go:16–68`; `internal/llm/types.go:193–213`; `internal/usage/store.go:1–29,44–60` | Completed-round stats omit cancellation; usage lacks account/window cost identity; usage store is picker history | None is a complete financial or quota ledger |
| `internal/tui/controller/runtime.go:28–54`; `internal/provider/storage.go:28–48` | Shared manager within runtime, credential entry per provider ID | Cross-process account reservations and stable identity are new work |
| `internal/agent/engine.go:626–672,1039` | No authoritative actual-tier notification today | Add first-call and post-switch harness-owned metadata |
| `doc/agent-backends.md:1–4,47–65` | External subscription backends are proposed | Do not claim Claude subscription integration exists |

## Existing related work

- [Model-specific OpenAI/Codex efforts](../obsidian-tasks/model-specific-openai-codex-efforts.md)
  is **todo**: resolve real capabilities, distinguish default and explicit levels,
  verify wire values; do not assume external labels map to native enums.
- [Executor quality evaluation](../obsidian-tasks/executor-quality-evaluation.md)
  is **blocked**: model, native effort and context must be disclosed separately;
  passing samples do not establish universal tier equivalence. Related evaluation,
  not a routing prerequisite or an automatic source of user tiers.
- [Executor context budget](../obsidian-tasks/executor-context-budget.md) is
  **blocked**: context overflow/compaction approval is separate from subscription
  budget. Routing must preserve that authority; this epic does not implement it.
- [Agent plan effort-only](../obsidian-tasks/agent-plan-effort-only.md) is **done**:
  the existing human-owned model-selection contract is the compatibility baseline.
- [Claude economy and ledger](../obsidian-tasks/claude-economy-threads-brakes-ledger.md)
  is **todo** and backend-specific, not an existing general account ledger.

Statuses describe this planning snapshot; consult the registry before starting.

## Unverified provider assumptions

Verify stable non-secret account identity across aliases, refresh and processes;
window applicability/sharing; capability/default effort behavior; observation
freshness and delay; reset semantics; external consumption; failed/cancelled/retry
charges; hidden paid overage; and enforceable price bounds. If not established,
retain unknown state and restrict automatic behavior instead of fabricating data.

`doc/codex-usage.md:85–104` records a past opt-in GET probe of one account dated
2026-09-05. It is prior evidence, **not** a live check in this planning session,
nor proof of all account plans, effort capabilities, prices or reset spending.
Future live probes require separate user consent and an evidence record that
states date, auth kind, tested scope and limitations without secrets.

## Planning verification

This delivery changes documents and task notes only. Verification checks contract
coverage, task dependencies, links, ownership and whitespace. No Go test outcome,
provider integration result or routing performance is claimed by this record.

# Subscription-aware quality-tier routing

Status: approved planning contract; not implemented. The user approved the
requirements and the nine-slice breakdown on 2026-09-06.

## Problem Statement

A user has several model subscriptions with overlapping usage windows and a
personal ranking of model/reasoning combinations. Manual selection either saves
too aggressively and wastes useful allowance at reset, or consumes the capacity
needed for important work. Independent sessions and child jobs can spend the
same apparent balance. Provider percentages do not reveal a precise token or
money budget, and a model name alone does not describe execution quality.

Success means doing assigned work at the required quality, protecting future
important work, and using credible surplus to improve useful work before reset.
It does not mean spending every allowance regardless of value.

## Solution

An explicitly enabled routing mode selects an **execution profile**: a model
paired with its actual **native effort**, assigned exactly one **quality tier**
by the user. Tiers are ordered `low < medium < high < xhigh`; they are personal
judgments, not claims of interchangeable benchmark performance.

A setup wizard presents available models and verified effort capabilities. The
user assigns tiers, permits participation, chooses a conversation default tier,
and configures initial reserve and optional paid spending. The harness chooses
profiles for main sessions and children using shared account consumption,
priority, protected reserve, credible surplus and latency. It explains its
choice and remains subordinate to explicit, scoped human decisions.

See [domain vocabulary](../CONTEXT.md#subscription-aware-routing),
[approved tickets](subscription-aware-routing-tickets.md) and the
[source-evidence record](subscription-aware-routing-evidence.md).

## User Stories

1. As a user, I want to rank model/native-effort pairs myself, so my preferences determine quality.
2. As a user, I want the wizard to show real effort capabilities, so labels do not promise unsupported behavior.
3. As a user, I want new models excluded until I rank and allow them, so discovery does not change my policy silently.
4. As a user, I want opt-in and explicit migration, so saved plans retain their original effort meaning.
5. As a user, I want a default required tier for conversation work, so routine requests need no routing instructions.
6. As a planner, I want to request a tier through effort, so I need not select provider-native reasoning settings.
7. As a parent, I want children to inherit required rather than upgraded actual tier, so upgrades do not inflate requirements.
8. As a user, I want selection at launch, step changes and unavailability, so routing follows meaningful events.
9. As a user, I want stable choices among close candidates, so the harness does not switch models every turn.
10. As a user, I want all local processes to share account commitments, so parallel launches cannot claim the same remainder.
11. As a user, I want external consumption and overlapping windows considered, so short-window surplus is not fictitious.
12. As a user, I want a configurable cold-start reserve that learns from demand, so important work retains capacity.
13. As a user, I want reserve released gradually near reset, so assigned work can benefit from expiring allowance.
14. As a user, I want useful surplus to raise actual quality, so subscriptions deliver value without invented busywork.
15. As a user, I want foreground latency considered, so a stronger profile does not make conversation needlessly slow.
16. As a user, I want important work ahead of ordinary active work and active work ahead of background work.
17. As a user, I want active upgrades allowed to delay ordinary background work, even at its minimum required tier.
18. As a user, I want activity based on my latest assignment rather than screen focus, so switching tabs is not reprioritization.
19. As a user, I want ordinary work to ask and wait when only protected reserve is available.
20. As a user, I want necessary work possible with unknown quotas, but no upgrades justified by imaginary surplus.
21. As a user, I want pins to prevent automatic replacement, even when a different profile appears preferable.
22. As a user, I want a disclosed, once-confirmed scoped exception for lower quality or reserve spending.
23. As a user, I want paid routing off by default and bounded when enabled, with a separate explicit cap exception.
24. As a user, I want missing prices or unenforceable monetary bounds to exclude automatic paid routing.
25. As a model, I want authoritative required tier, actual tier and native effort before my first and every post-switch inference.
26. As a user, I want persistent profile/tier visibility, brief switch reasons and optional candidate/quota explanations.
27. As a user, I want cancellations, retries and restores to preserve permissions and conservative consumption accounting.
28. As a user, I want OpenAI OAuth/Codex and Z.AI Coding Plan supported without pretending a Claude subscription backend exists.
29. As a user, I want simulations to reveal unjustified saving and authority violations, not just lower average cost.

## Implementation Decisions

### D1. Versioned meaning, profiles and configuration

- The new opt-in contract reuses model-facing plan and child-spawn `effort` as
  **required tier**, accepting only `low`, `medium`, `high`, `xhigh`; omitted or
  empty inherits. Native effort belongs to the selected execution profile.
  `none`, `minimal`, `max` remain legacy-native values; never normalize them into
  tiers. Validation errors name the active mode and supported values.
- Persist the interpretation/version with plans, jobs and restored sessions.
  Enabling routing does not reinterpret existing artifacts. Migration previews
  ambiguous values, retains pins, and requires agreement; no guessed mappings.
  A model-facing schema/help projection describes the artifact's actual mode.
  Mixed-mode child launches require explicit interpretation, not ambient drift.
- User-assigned tiers are independent of the task registry's `model_level`
  vocabulary (including `very_high`). No implicit conversion is authorized.
- Profiles have stable identity and a revision. Route bindings additionally
  identify provider, authentication kind and subscription account: identical
  model names on different accounts are not interchangeable budget identities.
- Verify effective/wire effort, including variant/options precedence, not just
  picker labels. Provider-default or no-effort profiles are eligible only when
  their effective behavior is sufficiently characterized; unknown or ignored
  effort must not be advertised as a verified explicit setting. Changed model
  capabilities or default behavior invalidate affected profile assumptions and
  require review. No invented alias such as external `ultra` meaning `max`.
- No automatic ranking, workload specialization or automatic model-authored
  profile/pin editing in V1. The wizard only inspects permitted metadata; it
  never spends inference credits to rank models without separate user consent.

### D2. One routing and admission module

- Put selection, account admission, reservations and explanations behind one
  deep module shared by main and children, not separate UI/provider policies.
  Its interface accepts work intent and returns **admit**, **wait**, **ask**, or
  **unavailable**, with a reason and, for admit, a selected profile and reservation.
  Completion/reconciliation closes or adjusts the claim idempotently.
- Work intent includes required tier, user-owned priority, activity identity,
  interaction/background character, current profile, pins, authorized scopes
  and technical needs such as context capacity and tools. Selection cannot
  silently discard required context, weaken tools/permissions or approval duties.
- OpenAI and Z.AI quota observations are two real adapters at the provider seam.
  Account identity and credentials remain provider-owned; callers receive only
  non-secret identities and normalized evidence. UI renders decisions, not policy.
- Model/effort changes take effect at startup, a plan-step transition, or an
  unavailability event, never halfway through an inference/tool round. Admission
  is renewed for subsequent chargeable calls without gratuitous model changes.
  A pending upgrade waits for an allowed selection event.
- All plan entry paths must agree: implicit and explicit start, resume, settle,
  restoration and completion back to conversation defaults. Child preparation
  is shared by interactive and headless runs. A delayed spawn uses the required
  tier/priority/authority snapshot of its originating round, then current budget
  evidence when it actually reserves consumption. Revalidate grant expiry and
  revocation at dispatch; an originating snapshot cannot resurrect stale consent.
- Unsupported settings, stale pins and incompatible context produce actionable
  failures or user choices, not silent fallback. Unpinned unavailability may
  select another eligible profile. Distinguish overload/throttling from exhausted
  quota and authentication failure. Respect retry timing and cancellation;
  bound retries, and never replay already-applied tool effects during rerouting.

### D3. Account evidence and consumption reservations

- Coordinate across all local CozyPhi processes using the same account, including
  different workspaces and provider aliases. Credential refresh does not create
  fresh budget; account switch does. Do not use raw credentials as public keys.
  Unknown account identity cannot be treated as independent free capacity.
- Observe every applicable account/model/shared window, unit, remaining amount
  or percentage, reset horizon, observation time, presence and confidence.
  Missing, stale, failed and zero observations are distinct. Percentages cannot
  be converted into exact tokens, money or unrelated token-scope totals.
- Admission atomically reserves expected consumption in all applicable windows
  before inference. Estimates remain estimates, with conservative uncertainty
  margins; provider enforcement, not local estimates, determines hard quota.
  Failed coordination is not permission to create an uncoordinated reservation.
- Track every chargeable attempt: main, children, compaction, retries, failures
  and cancellations. Reconcile reported usage without counting both a pending
  reservation and its included provider observation. Snapshot timestamps and
  accounting watermarks must prevent stale readings from restoring spent credit.
- Reservations have ownership, bounded lifetime and recovery. Process death,
  duplicate completion, out-of-order observations and reset rollover must not
  leak claims indefinitely or refund uncertain remote charges as free allowance.
  Uncertain in-flight consumption stays conservative until reconciled; reclaiming
  a dead worker's unused claim differs from assuming its request was uncharged.
- External devices/apps are not locally locked. Detect observed external drain,
  revise available headroom and explain uncertainty; do not claim to prevent it.
  Account telemetry contains no credentials or unrelated session content.

### D4. Deterministic selection policy

Use ordered constraints rather than a single score that trades away authority:

1. Resolve versioned required tier, inherited user priority and explicit pins.
   Validate technical feasibility, participation and existing scoped permissions.
2. Evaluate account evidence and commitments across every applicable window.
   Remove unavailable routes and automatic paid routes without valid authorization.
3. Schedule important work before ordinary active work before ordinary background
   work. Activity is the latest user assignment, not focus; store a consistent
   ordering across local processes. User changes on a step/job override inherited
   session priority. The model may propose importance, never grant it to itself.
4. Admit at least the required tier without consuming protected reserve for
   ordinary work. Important work may use that reserve. A higher profile that is
   the lowest available way to meet required quality is necessary execution, not
   an upgrade justified by speculative surplus. With unknown quotas, necessary
   work remains possible under conservative admission and applicable permissions.
5. If credible useful surplus remains, consider higher-tier eligible profiles
   for already-assigned work. Background work tolerates upgrades more readily;
   interactive work also respects configurable latency tolerance. An active
   upgrade may delay ordinary background admission even at its required minimum.
6. Among similarly useful admissible choices, keep the current profile; use a
   deterministic tie-break when none is current. Never automatically downgrade
   below required tier. Returning from a previous upgrade to required tier is
   permitted at a selection event and explained.
7. If only protected reserve remains for ordinary work, ask and wait. If no
   required-tier route is feasible, explain wait/alternative/exception choices.
   Never silently enable paid routing or ignore a pin. A wait is cancellable and
   wakes on relevant budget/reset/permission changes, not busy polling.

Priority controls future admission, not forced cancellation of an in-flight
round. Background starvation under continuous higher-priority demand is visible;
aging may order equals, but must not silently invert the agreed priority policy.
A headless caller unable to answer receives a structured pending decision rather
than consent inferred from silence.

### D5. Reserve, surplus and uncertainty

- Begin with a configurable reserve and conservative workload estimate. Adapt to
  observed important demand, concurrency, forecast uncertainty and remaining time
  to reset. Release gradually as the protected horizon contracts; do not release
  a long/shared-window reserve just because a short window is about to reset.
- For each window, evaluate observed remaining allowance minus unreflected
  consumption claims, expected assigned demand, protected reserve and uncertainty
  margin. This is a planning estimate in that window's native unit, not exact
  tokens. A route has credible surplus only if all applicable windows support it.
  Avoid subtracting the same expected work in both demand and reservations.
- Missing/stale observations, an uncertain reset, or uncalibrated conversion do
  not establish useful surplus. Necessary work may continue, but unknown data
  cannot justify burn-before-reset upgrades. A past reserve estimate cannot
  manufacture a currently known balance.
- Thresholds, freshness limits, history horizon, latency tolerance and stability
  hysteresis are configurable, bounded and calibrated with deterministic traces.
  V1 does not claim an optimal numerical forecast before that evaluation.
- Never generate extra tasks, retries, reviews or longer outputs solely to spend
  allowance. Unused quota is not itself failure: failure is an explainable missed
  chance to improve assigned work without violating the contract. Zero expiry
  and guaranteed reserve cannot both be promised under unknown future demand.

### D6. Human authority and paid spending

- A user pin prohibits automatic replacement. It can coexist with an explicitly
  confirmed exception allowing actual tier below required or use of reserve.
  Merely selecting a risky pin is not undisclosed blanket permission: explain
  the specific exception once, obtain the user's confirmation, then honor it
  without repeated prompts within the confirmed scope.
- Permission records identify the affected rule, account/profile, work scope,
  duration or expiry and any limit. Revocation or material scope changes require
  re-evaluation. Restart preserves valid scope, not expired/revoked authority.
  Plan text, model arguments, tool results and files cannot impersonate approval.
- Paid inference is off by default. Opt-in requires an explicit monetary ceiling
  and scope, including concurrent processes covered by the grant. Reliable price
  data and an enforceable conservative bound on chargeable calls are prerequisites
  for ordinary automatic paid routing. Reserve money before dispatch and reconcile
  all attempts; do not present post-hoc usage tracking as a hard cap.
- If price or upper-bound enforcement is unavailable, exclude the route from
  ordinary automatic paid selection. The user may separately confirm a concrete
  cap/unknown-cost exception after disclosure; it is not inferred from a routing
  pin or previous consent to paid calls. Record the bounded scope and remaining
  risk accurately. No promise that software can undo provider charges.
- These exceptions override routing policy, not technical impossibility, provider
  restrictions or current tool security gates. General user root-override of
  tool restrictions is a separately linked task, not part of this epic.

### D7. Model awareness and user explanations

Before the first inference and before the next inference after a profile/effort
change, supply a harness-owned authoritative record of required tier, actual tier,
model/profile, effective native effort and any scoped exception. A requested tier
is not proof of actual quality. Historical records remain historical; restore and
compaction must not leave a superseded statement authoritative. Updates match the
same immutable execution snapshot used for inference and child launch.

Keep current profile and actual tier visible. Distinguish pending selection from
currently executing selection. Explain switches briefly; on demand show candidate
rejections, relevant windows/freshness, reserve/surplus estimates, commitments,
priority and permissions. Do not expose credentials or other assignments' content.
Every ticket includes its observable UI/headless output and relevant regression
coverage rather than deferring all UX to a final horizontal task.

## Testing Decisions

The primary test seam is the shared routing/admission interface with controlled
time, observations and request outcomes. Assert admitted profile, wait/ask reason,
reservation totals and authoritative execution state, not private implementation
structure. Provider contract fixtures and engine round/child tests verify the two
ends of that interface. Existing quota fixture tests, model/effort selection tests
and immutable round snapshots are prior art; existing UI usage is not a ledger.

The following fixed scenarios are acceptance obligations, not claims of tests
already executed. Every scenario records initial state, time, events, decision,
reason, remaining claims and authoritative model metadata.

| ID | Scenario | Required result |
|---|---|---|
| S01 | Enable tier mode with legacy saved plan/job and `minimal/max` | Old meaning retained; migration explicit; no silent coercion |
| S02 | Model label says high; effective variant differs or effort is ignored | No false verified profile/tier; wire behavior or actionable exclusion |
| S03 | Parent requires medium, runs xhigh, then spawns delayed child; grant expires or is revoked before dispatch | Child inherits medium, reserves against current evidence and revalidates originating grants; stale consent cannot authorize execution |
| S04 | Two processes share account via aliases; simultaneous admission | Atomic claims across all windows; no independent double allocation |
| S05 | Short-window reset soon, long/shared window nearly exhausted | No upgrade using fictitious short-window surplus |
| S06 | Cold start; then predictable demand and imminent reset | Configured reserve first; bounded adaptation and useful release later |
| S07 | Credible surplus and assigned ordinary work | Higher useful profile selected where latency allows; no manufactured work |
| S08 | Missing, stale, malformed or delayed quota; zero vs absent | Necessary work remains possible; no speculative surplus upgrade |
| S09 | External consumer drains account after local observation | Reconcile conservatively; explain reduced headroom, no exact-budget guarantee |
| S10 | Important, active and background queue; focus changes | Agreed priority preserved; active upgrade may delay background minimum; focus irrelevant |
| S11 | Ordinary work can only consume reserve | Ask and cancellable wait; user-approved scoped exception runs without re-asking |
| S12 | Pin below required; stale pin; model tries to approve exception | Disclose/confirm first case; no automatic replacement; reject forged consent |
| S13 | Paid route disabled, cap nearly used, concurrent retries | No default spending; atomic monetary bounds include all attempts |
| S14 | Missing prices/unbounded charge; explicit cap exception | Exclude normal auto route; run only with genuine scoped exception, disclose uncertainty |
| S15 | Process dies during inference; duplicate completion; reset/refresh | No double refund/charge or infinite claim leak; account identity remains correct |
| S16 | Start/resume/settle/restore/complete and interactive/headless child | Same selection rules; no mid-round rebind; current tier record precedes inference |
| S17 | Quota exhaustion vs overload/auth error vs partial stream | Correct reason, bounded recovery, no duplicate tool effects or silent downgrade |
| S18 | No assigned work; quota expires | No synthetic spending; leftover alone is not reported as policy failure |
| S19 | Continuous active demand starves background | Waiting and cancellation visible; no hidden priority inversion |
| S20 | Different workspaces, account switch, replayed approval, restart | Identity isolation and scope/revocation respected; no secret leakage |

A deterministic trace suite measures unmet required quality, unauthorized spend,
reserve breaches, missed useful upgrades, latency and waiting by priority, churn
and prediction error. Authority/quality invariants outrank savings metrics.
Replay the same workloads against a fixed required-tier baseline and the adaptive
policy; explain differences rather than asserting that one always wins. Include
adversarial traces, not only favorable workloads. Record all policy parameters.

Default verification is offline with fixtures and synthetic inference. Provider
metadata/GET probes and any bounded live inference are separate, explicitly
user-authorized verification steps, reporting account/date/auth method, scope and
limitations without credentials. Evidence from source or one account is not live
verification of all subscriptions. Unverified provider assumptions limit rollout.

## Out of Scope

- Implementing any code as part of this planning task, changing the current model,
  reading credentials, or making live inference/quota requests now.
- A new Claude subscription backend, additional account-connection UX, universal
  subscription support, cross-device locking, or reliable absolute token balances
  derived from percentages. Existing connected OpenAI OAuth/Codex and Z.AI Coding
  Plan are the V1 integrations; other providers stay manual/conservative.
- Automatic user-tier ranking, task-type specialization, a general model-authored
  pipeline/DAG language, unrelated executor redesign, or extra work to burn quota.
- Automatic purchase of reset credits or hidden paid overage. Manual reset credit
  confirmation remains separate from adaptive reserve and routing permissions.
- Implementing general tool root-override; its separate ticket must preserve the
  distinction between human authority and an agent's claims about that authority.

## Further Notes

The architecture favors one deep routing module for locality and testability,
with two real quota adapters. Reliability requires cross-process commitments and
uncertainty handling rather than extending UI percentages into a guessed ledger.
Extensibility does not justify speculative backends. Security keeps human consent
explicit; readability keeps required tier, actual tier and native effort separate.

Reusing `effort` was an explicit user choice over introducing a separate tier
field. Its compatibility cost is accepted through versioned opt-in, artifact-local
interpretation and explicit migration, not through global renaming.

Open implementation questions are evidence/calibration work, not permission to
change this contract: stable provider account/window identity, hidden overage,
real effort/default behavior, quota freshness and attribution, enforceable paid
bounds and numerical forecast parameters. Escalate a contradiction rather than
silently weakening required quality or authority. Existing executor context-budget
approval remains in force; routing does not replace context/compaction consent.

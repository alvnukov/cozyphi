# Subscription-aware routing: approved implementation tickets

The user approved this granularity, dependency graph and test seam on 2026-09-06.
Implementation has not started. [Contract](subscription-aware-routing.md) ·
[Evidence limits](subscription-aware-routing-evidence.md) ·
[Epic](../obsidian-tasks/subscription-aware-routing.md)

Each slice includes the configuration, behavior, UI/headless explanation and
public-interface tests needed for its own demonstration. Slice 09 adds whole-flow
verification, not the first tests or UI. Task notes contain full acceptance and
verification criteria; numeric prefixes are presentation labels, not execution
order. Work the dependency frontier, not simply the number sequence.

| Slice | Task | Blocked by | End-to-end delivery |
|---|---|---|---|
| 01 | [Versioned tier profiles](../obsidian-tasks/routing-tier-profiles.md) | [Existing model-specific efforts](../obsidian-tasks/model-specific-openai-codex-efforts.md) | Wizard, explicit migration, trustworthy profile and first-call tier metadata |
| 02 | [OpenAI account admission](../obsidian-tasks/routing-openai-account-admission.md) | 01 | Main selection and atomic account commitments across local processes |
| 03 | [Z.AI account admission](../obsidian-tasks/routing-zai-account-admission.md) | 02 | Second real quota adapter and cross-provider selection with honest uncertainty |
| 04 | [Adaptive reserve and surplus](../obsidian-tasks/routing-adaptive-reserve.md) | 02 | Important-work reserve and useful upgrades before reset, protecting all windows |
| 05 | [Priority scheduling](../obsidian-tasks/routing-priority-scheduling.md) | 04, 06 | Important → active → background admission; active upgrades may delay background minimum |
| 06 | [Children and lifecycle](../obsidian-tasks/routing-children-lifecycle.md) | 02 | Required-tier inheritance, safe step/child switching, recovery and current model awareness |
| 07 | [Human pins and exceptions](../obsidian-tasks/routing-human-exceptions.md) | 02 | Genuine once-per-scope confirmation, pin precedence and no forged consent |
| 08 | [Paid monetary admission](../obsidian-tasks/routing-paid-budget.md) | 07 | Default-off paid routes, enforceable scoped caps and separately confirmed exceptions |
| 09 | [Simulation and rollout](../obsidian-tasks/routing-simulation-rollout.md) | 03, 05, 06, 07, 08 | Complete offline scenario matrix, comparative evidence and opt-in user guide |

An example valid order is: existing effort task → 01 → 02 → {03, 04, 06, 07}
→ {05 after 04+06, 08 after 07} → 09. Slices 06 and 07 are explicitly listed
as rollout gates, though also reachable transitively through 05 and 08.

## Acceptance coverage

| Contract | Primary owners | Verification obligations |
|---|---|---|
| D1: profile truth, versioned effort, migration | 01 | S01–S03; catalog/wire and legacy fixtures |
| D2: shared admission and safe execution snapshots | 02, 06 | S03–S04, S15–S17; real child/engine seams |
| D3: account/window evidence and all-attempt reconciliation | 02, 03 | S04, S08–S09, S15, S20; two-process races |
| D4: ordered selection and priority | 04, 05, 06 | S05–S12, S16–S19; deterministic decisions and cancellable waits |
| D5: reserve, useful surplus and uncertainty | 04 | S05–S09, S11, S18; fixed baseline/adaptive traces |
| D6: human authority and paid scope | 07, 08 | S11–S15, S20; impersonation, revocation and cost-bound tests |
| D7: pre-inference awareness and explanations | 01, 06; every slice's visible decisions | S02–S03, S16; prompt/round snapshot and UI/headless checks |
| Full opt-in user journey and readiness limits | 09 | All S01–S20, evidence report and migration/restore walkthrough |

The primary interface is admit / wait / ask / unavailable followed by idempotent
consumption reconciliation. Test observable selected profile, permissions,
account claims, reason and pre-inference metadata. Provider fixtures validate real
wire semantics, and engine tests exercise actual lifecycle integration.

## Separate related work

[General user root-override](../obsidian-tasks/user-root-tool-override.md) is a
standalone task, linked to slice 07 but not a child or prerequisite. It must design
which tool restrictions can be explicitly overridden, how concrete risk and scope
are confirmed once, and which technical/security restrictions remain. No current
permission change or destructive operation is authorized by creating the task.

Existing [executor quality evaluation](../obsidian-tasks/executor-quality-evaluation.md)
and [executor context budget](../obsidian-tasks/executor-context-budget.md) remain
independent. The former does not rank profiles for the user; the latter's context
and compaction approval is not replaced by subscription routing.

## Planning delivery versus implementation delivery

[Planning task](../obsidian-tasks/subscription-routing-spec.md) closes when this
specification, vocabulary, evidence and registry are reviewed and committed.
The epic and all implementation slices remain open. Dependency-bearing slices
are blocked until their named prerequisites complete; the registry's human-readable
blocking edges are part of the contract, not native executable DAG semantics.
No implementation gate or live-provider result is claimed by this planning work.

# Harness security: approved delivery slices

The user approved these 13 slices, blocking edges and public test seams on
2026-09-09. [Standalone contract](harness-security.md) ·
[Planning task](../obsidian-tasks/harness-security-spec.md) ·
[Unchanged parent epic](../obsidian-tasks/harness-security-hardening.md)

Every implementation slice includes its own configuration/state changes,
UI/headless explanation and public-interface tests. Slices 07 and 13 are reviewed
architecture deliverables, not production implementations. Slice 12 is whole-flow
verification, not the first tests. No implementation has started through this PR.

## Dependency frontier

Numbers are presentation labels, not a required execution order. Follow the named
blocking edges. Registry dependencies are human-readable `Blocked by` lists, not
a native executable DAG. Tasks with prerequisites are marked blocked; tasks with
none are todo. Design publication does not complete an implementation prerequisite.

| Slice | Registry task | Blocked by | Independently demonstrable delivery |
|---|---|---|---|
| 01 | [Opt-in foundation](../obsidian-tasks/security-opt-in-foundation.md) | None | Off compatibility and first confidential read-to-model recipient decision |
| 02 | [Durable provenance](../obsidian-tasks/security-durable-provenance.md) | 01 | All ingress retains restrictions through resume, compaction and delegation |
| 03 | [Every model egress](../obsidian-tasks/security-model-egress.md) | 02 | Actual model payload and recipient checks across all roles and attempts |
| 04 | [Bound action grants](../obsidian-tasks/enforce-agent-dataflow-policy.md) | 02 | Final-argument and recipient-bound decisions across visible harness actions |
| 05 | [Observe guard](../obsidian-tasks/security-observe-guard.md) | 03, 04 | Isolated cheap guard observations, safe quarantine/audit and technical bounds |
| 06 | [Confirmed escalation](../obsidian-tasks/security-confirmed-escalation.md) | 05 | Strong-model second opinion only after explicit permitted-data confirmation |
| 07 | [Storage design](../obsidian-tasks/security-storage-design.md) | None | Accepted encryption/key/migration/recovery contract and artifact inventory |
| 08 | [Encrypted sessions](../obsidian-tasks/security-encrypted-sessions.md) | 02, 07 | Encrypted sensitive lifecycle and derivatives, including off-mode continuation |
| 09 | [Trusted profiles](../obsidian-tasks/security-trusted-profiles.md) | 03, 08 | All-role trust and explicit clean/redacted/cancel untrusted transfers |
| 10 | [Adversarial and benign evals](../obsidian-tasks/agent-security-adversarial-evals.md) | None | Offline controlled-sink corpus and honest task/outcome measurement interface |
| 11 | [Opt-in enforcement](../obsidian-tasks/security-opt-in-enforcement.md) | 06, 09, 10; accepted readiness evidence | Quarantine/pause/resolution journey without detector-issued authority |
| 12 | [V1 readiness](../obsidian-tasks/security-v1-readiness.md) | 11 | Full mode walkthrough, comparative evidence and explicit uncovered threats |
| 13 | [Process-isolation design](../obsidian-tasks/security-process-isolation-design.md) | None | Reviewed second-stage process/network/environment containment contract |

An example valid order is {01, 07, 10, 13} → 02 → {03, 04, 08 after accepted 07}
→ {05 after 03+04, 09 after 03+08} → 06 → 11 after 06+09+10 and accepted evidence
→ 12. Independent tasks need not wait for unrelated branches of that order.

Slice 10 can start with scripted providers; later feature slices add their cases.
Slice 11 requires a user-accepted attack-success threshold, model/version and
appropriate evaluation evidence in addition to completed code dependencies.
These are unresolved readiness decisions, not values an implementer may invent.
Candidate behavior can be evaluated offline before enabling the runtime mode;
no enforcement/eval dependency cycle is intended. Live or paid trials require
separate approval. Slice 07 completion includes acceptance by the user or an
explicitly authorized human reviewer before 08 starts; an agent review alone does
not satisfy that gate.

## Contract and test coverage

| Contract | Primary owners | Public verification |
|---|---|---|
| D1: off, observe, enforce, non-model authority | 01, 11; every slice's off behavior | S01, S02, S13; engine/UI/headless traces |
| D2–D3: confidentiality and durable provenance | 01, 02 | S03, S04; real ingress, session and child adapters |
| D4: bound grants, final arguments, process limits | 04, 13 | S05, S06, S17; actual executor and controlled sinks |
| D5: every model dispatch and retained context | 03, 09 | S07, S14; full fake-provider payloads and transport attempts |
| D6: guards, quarantine, escalation and safe audit | 05, 06, 11 | S08–S10, S15; scripted guard outputs and synthetic canaries |
| D7: trusted identity and encrypted artifacts | 05, 07, 08, 09 | S10–S13, S15; profile and public storage lifecycle |
| D8: untrusted transitions | 09 | S14; recipient changes and clean/redacted/cancel outcomes |
| D9: measurable readiness, cost and limits | 10, 11, 12 | S16 plus S01–S17 whole-flow walkthrough; uncertainty and coverage |

Prefer existing engine/executor/provider/session seams. The small policy interface
owns decisions; tests observe permitted/refused dispatches and actions, retained
restrictions, encrypted artifacts and safe explanations, not private structures.
Each code slice runs only changed-package checks locally with at most one scoped
lint before commit; full repository gates belong to CI. Document-only design
slices need document/schema/link review, not Go tests.

## Existing work and publication boundary

Reuse [dataflow policy](../obsidian-tasks/enforce-agent-dataflow-policy.md) and
[adversarial evals](../obsidian-tasks/agent-security-adversarial-evals.md). Their
original parent and creation timestamps remain intact. Provenance previously
included in the broad dataflow task is now its explicit slice-02 prerequisite;
this does not remove that requirement. Existing deterministic exploit regressions
remain part of the eval task.

The process design links [MCP environment/sandbox work](../obsidian-tasks/sandbox-mcp-stdio-environment.md)
and the [managed subprocess seam](../obsidian-tasks/refactor-external-binary-runner.md)
without duplicating their implementation or modifying their notes. The umbrella
epic and all unrelated tasks remain unchanged. No promise of process-egress DLP
is made by V1, and process-design completion is not sandbox delivery.

This PR contains the specification, this map, 11 new delivery-slice notes, updates
to two existing notes, and one planning note. The user explicitly authorized
branch-local markdown writes because the current native task tool cannot select
a worktree. No task write is made on main. Publication is authorized; merge and
feature implementation are not. The planning note stays in progress until the
review/merge boundary, and all delivery slices remain open.

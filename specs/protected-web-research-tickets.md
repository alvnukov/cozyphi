# Protected web research — approved delivery tickets

Status: the user approved this 46-ticket decomposition on 2026-09-19 through the
`to-tickets` workflow. Publication is to the local task registry only. This is a
backlog and evidence contract, not an executable plan or authorization to implement,
run live model trials, publish remotely, merge, or close the parent epic.

Source: [confirmed specification](protected-web-research.md), Q1–Q43 and T01–T32.
Parent: [web-tools](../obsidian-tasks/web-tools.md), unchanged by this breakdown.
Authoring: [protected-web-research-tickets](../obsidian-tasks/protected-web-research-tickets.md).
Baseline: specification commit `aed5af09`, based on `4a339857`.

There are **15 low and 31 medium** delivery tickets. Initially only 01 and 02 are
`todo`/`ready-for-agent`; 03–46 are `blocked`. A blocked note is not an invitation
to implement its dependencies. Reopen it only after its named dependencies and
additional contract/evidence/consent gates are actually satisfied.

## How an executor uses one ticket

1. Read that ticket, its cited specification decisions/scenarios and completed
   blocker evidence. Use the registry's `current`, then `get`; verify the actual
   checkout and landed revisions rather than trusting an old status.
2. Check that its public APIs and prerequisite contracts exist. Locate actual
   definitions/references with LSP; do not assume a filename, function signature,
   provider field or OS primitive. The tickets deliberately avoid speculative
   source paths and APIs.
3. Start only the chosen ready task in its isolated branch/worktree. Each is one
   narrow behavior or bounded verification/report deliverable, not a license to
   redesign the common security architecture. Low tasks use fixed contracts and
   fixtures. Medium tasks integrate already approved seams.
4. Implement the smallest complete user-observable path, including its failure
   outcome, host integration, UI explanation where relevant and public-boundary
   regression. Tests belong with the code. Production enablement remains closed
   whenever a required safety prerequisite is missing; test injection is never a
   user-accessible bypass.
5. Do not infer that an existing interface already implements a guarantee. If a
   required shared contract is absent, ambiguous or too large for one context,
   stop, record the exact blocker and request separately sized prerequisite work.
   Do not relabel an unresolved architecture task as medium and improvise it.
6. Provider/protocol/parser changes require authoritative service source/spec
   evidence and comparison with an established implementation before coding.
   Neither a model name nor a single observed payload establishes a protocol.
   The selected web model is always user-configured; no provider is mandated.
7. Record reproducible evidence, obtain required review and commit only owned
   files. User-visible runtime changes include a changelog entry. Task completion
   follows the repository PR/review policy; a local commit is not merged delivery.
   Never close the parent epic from a child task.

## Verification and required proof packet

Every note contains its own acceptance criteria, verification recipe and
**Proof required** paragraph. These common rules supplement, not replace, them.

- **Revision and scope:** task ID, tested commit/tree, changed packages, effective
  policy/model configuration identity without credentials, platform and fixture
  versions. A report referring to another revision is not current proof.
- **Reproduction:** exact executed commands, prerequisites, relevant environment
  and observed exit/results. Supply committed public-boundary fixtures or safe
  reproducible artifacts, not only a screenshot or an assertion that tests passed.
- **Positive and negative evidence:** at least the promised successful behavior
  and its important refusals. Observe actual consumer messages, provider payloads,
  action/network/filesystem sinks or owned-process outcomes as the task requires.
  A returned `deny` value alone does not prove no action occurred.
- **Regression sensitivity:** for a bug/guard regression, show failure on the old
  behavior or a temporary controlled bypass of the relevant guard, then a passing
  run with protection restored. Never commit fault-injection changes. Report when
  a safe sensitivity demonstration is unavailable instead of inventing it.
- **Concurrency:** synchronization barriers, controlled completions and fake clocks
  establish order. No sleeps or retries-until-pass as correctness evidence. Use
  scoped race tests where the changed behavior is concurrent.
- **Privacy:** only synthetic secrets and controlled attack destinations. Evidence
  containing sensitive payloads stays in protected storage; routine logs/task notes
  contain safe identities, outcomes and redacted references.
- **Limits:** distinguish pass, fail, skipped, unavailable, unproved and incomplete.
  Fake model replies prove harness behavior, not live model detection quality.
  Model pass is not trust, and a finite attack suite is not a universal guarantee.
- **Live work:** only task 45's separately consented protocol (or another explicitly
  approved bounded trial) authorizes model spend. Creating these tasks authorizes
  no live calls, browser sessions or attacks on user/third-party resources.

### Scoped commands, not repository sweeps

Resolve the actual changed files/packages from the task's git diff; do not invent
a package or test name in advance. Record the expanded commands in the proof:

```text
git diff --check
golangci-lint fmt <changed-package-paths>
gofmt -l <changed-go-files>
go build <changed-package-paths>
go test <changed-package-paths>
go test -race <changed-concurrent-package-paths>
golangci-lint run <changed-package-paths>
```

The last command runs **at most once per task, immediately before the final
commit**. Use exact paths, not `./...`; do not run `make fmt`, `make lint`,
`make test` or a final whole-repository sweep. Platform subprocess tests may need
a targeted integration command from the existing runner; record the command and
actual platform. Do not repeat already accepted broad checks to finish a report.
For documentation-only tasks 01, 36 (unless a separately approved fixture changes
code), and 46, validate content/links/diff only; run no Go gates. Whole-repository
checks belong to CI. A failed gate is reported, not hidden by a pipeline exit code.

## External prerequisites and unresolved gates

These are reused owners, not duplicate implementation tickets:

| Key | Existing owner | Required distinction |
| --- | --- | --- |
| P | [security-durable-provenance](../obsidian-tasks/security-durable-provenance.md) | Actual shared propagation, including explicit fork integration; a transient taint flag is insufficient. |
| E | [security-model-egress](../obsidian-tasks/security-model-egress.md) | Effective requests to all model recipients; search/site recipients additionally use the web research grant. |
| G | [enforce-agent-dataflow-policy](../obsidian-tasks/enforce-agent-dataflow-policy.md) | Actual final arguments and bounded user grants, not merely proposed model actions. |
| S | [security-storage-design](../obsidian-tasks/security-storage-design.md) and [security-encrypted-sessions](../obsidian-tasks/security-encrypted-sessions.md) | Reviewed design plus delivered shared storage; web specifically selects OS-backed keys, not silent passphrase fallback. |
| A | [routing-openai-account-admission](../obsidian-tasks/routing-openai-account-admission.md) | Owner of the shared admit/claim/reconcile foundation; the first provider adapter does not choose the web provider. A compatible proven adapter for the user's selected route is additionally required. |
| R | [routing-priority-scheduling](../obsidian-tasks/routing-priority-scheduling.md) and [routing-human-exceptions](../obsidian-tasks/routing-human-exceptions.md) | Delivered shared priority admission and unchanged user pin, beyond foundation A; task names do not prescribe the web provider. |
| I | [security-process-isolation-design](../obsidian-tasks/security-process-isolation-design.md), then concrete channel/platform implementation and proof | The design task is NOT an implementation. 01/36 must identify real landed implementation IDs before dependent runtime tickets reopen. |
| H | Shared host-coordination owner identified by 01 | No completed cross-process block/release implementation is assumed. 27 must record the approved owner and delivered contract first. |

Related owners include [managed subprocesses](../obsidian-tasks/refactor-external-binary-runner.md),
[MCP environment](../obsidian-tasks/sandbox-mcp-stdio-environment.md) and
[shared security evals](../obsidian-tasks/agent-security-adversarial-evals.md).
A managed runner, scrubbed environment or an eval task name does not prove isolation.
Reuse their established contracts when available; do not make unrelated full-program
completion a blanket blocker for every web task.

I and H are **explicit unresolved evidence gates**, not fictitious task IDs and not
completed prerequisites. The graph is intentionally not fully executable until
owners/designs are resolved. New prerequisite implementation work must be separately
specified/reviewed; it is outside this approved 46-ticket authoring scope. Once
identified, replace each applicable unresolved gate in its task with concrete IDs
and proof references before reopening. No dependent executor may fill the gap by
inventing a sandbox, shared database or cryptographic stack.

For I, a concrete adapter/channel can be unavailable while independent proven paths
continue. Safe bounded nonpersistent work does not wait for S. Missing core isolation
keeps production web unavailable. Task 41 verifies/assembles the restricted mode;
it is **not** a prerequisite implementation of I for task 11, which would introduce
a cycle. The actual common isolation implementations must precede those consumers.
Task 36 completes a reviewed feasibility report, not all platform implementations.

**Integration is not enablement.** Tasks 06 and 11 establish controlled offline
paths; subsequent consumers can integrate and test behind a closed production gate.
Task 41 owns aggregate runtime eligibility after mandatory budgets/account admission,
lifecycle/action grants, incident/block/revocation and human resolution are delivered.
It checks every enabled adapter's additional proof, including resource revocation
and persistence when applicable; missing optional methods remain unavailable.
Passing 11 alone never enables production research. Preflight has its own narrow
consent/admission boundary. Runtime eligibility still does not authorize rollout or
live spending; 45 requires separate consent and 46 reports actual acceptance evidence.

## Approved task map

Numbers refer to individual notes. Ordinary edges name work required to start;
additional I/H/adapter/consent gates remain blocking even when predecessors are done.
Review clarified dependency edges without adding feature scope: 04/06 use foundation
A; 23/38 consume budget 18; 40 consumes delivered revocation 29 and persisted reuse
35; 41 waits for mandatory protections. Regression 42 needs refresh 14 and 43 needs
source transfer 34. Classification 26 tests fixed events; real multi-source stage
integration is additionally verified by 42 after 17. Verification reuse 35 includes
checking-context compatibility, not only matching source bytes and policy versions.

| # | Task | Model | Blocked by |
| --- | --- | --- | --- |
| 01 | [Channel/platform evidence inventory](../obsidian-tasks/web-research-01-channel-inventory.md) | low | None |
| 02 | [Not-ready and no legacy bypass](../obsidian-tasks/web-research-02-not-ready.md) | low | None |
| 03 | [Explicit pinned model configuration](../obsidian-tasks/web-research-03-model-binding.md) | low | 02 |
| 04 | [Consented capability preflight](../obsidian-tasks/web-research-04-consented-preflight.md) | medium | 03, E, A |
| 05 | [Research data/recipient consent](../obsidian-tasks/web-research-05-research-consent.md) | medium | 03, E, G |
| 06 | [Single-source offline tracer bullet](../obsidian-tasks/web-research-06-single-source-tracer.md) | medium | 03, 05, A |
| 07 | [Invalid outputs and attempted tool calls](../obsidian-tasks/web-research-07-invalid-responses.md) | low | 06 |
| 08 | [Parallel release/cancel races](../obsidian-tasks/web-research-08-release-races.md) | medium | 06 |
| 09 | [Exact host-owned citations](../obsidian-tasks/web-research-09-exact-citations.md) | low | 06 |
| 10 | [Public-only acquisition](../obsidian-tasks/web-research-10-public-acquisition.md) | medium | 05 |
| 11 | [Disabled static-source integration](../obsidian-tasks/web-research-11-static-answer.md) | medium | 04, 08, 09, 10, P; I |
| 12 | [Question-driven search](../obsidian-tasks/web-research-12-question-search.md) | medium | 11 |
| 13 | [Immutable-source follow-up](../obsidian-tasks/web-research-13-source-followup.md) | low | 11 |
| 14 | [Explicit source refresh](../obsidian-tasks/web-research-14-source-refresh.md) | low | 13 |
| 15 | [Checked exact excerpts](../obsidian-tasks/web-research-15-checked-excerpts.md) | medium | 09, 13 |
| 16 | [Consumed-region coverage](../obsidian-tasks/web-research-16-consumed-coverage.md) | medium | 15 |
| 17 | [Independent multi-source partials](../obsidian-tasks/web-research-17-independent-partials.md) | medium | 12, 16 |
| 18 | [Total research budgets](../obsidian-tasks/web-research-18-research-budgets.md) | medium | 06 |
| 19 | [Shared account admission](../obsidian-tasks/web-research-19-account-admission.md) | medium | 18, R |
| 20 | [Safe user progress](../obsidian-tasks/web-research-20-safe-progress.md) | low | 06 |
| 21 | [Origin-bound delivery](../obsidian-tasks/web-research-21-origin-delivery.md) | medium | 08, 20 |
| 22 | [Exit and explicit continuation](../obsidian-tasks/web-research-22-explicit-resume.md) | medium | 18, 21 |
| 23 | [Requested checked partials](../obsidian-tasks/web-research-23-requested-partials.md) | low | 17, 18, 21 |
| 24 | [Durable web lineage](../obsidian-tasks/web-research-24-durable-lineage.md) | medium | 11, P |
| 25 | [Post-web effective-action grants](../obsidian-tasks/web-research-25-post-web-grants.md) | medium | 24, G |
| 26 | [Incident classification](../obsidian-tasks/web-research-26-incident-kinds.md) | low | 07 |
| 27 | [User-wide exact-host blocks](../obsidian-tasks/web-research-27-host-blocks.md) | medium | 26; H |
| 28 | [In-flight revocation](../obsidian-tasks/web-research-28-inflight-revocation.md) | medium | 24, 27 |
| 29 | [Previously delivered revocation](../obsidian-tasks/web-research-29-delivered-revocation.md) | medium | 25, 28 |
| 30 | [Human-only incident viewer](../obsidian-tasks/web-research-30-incident-viewer.md) | low | 20, 26 |
| 31 | [User recheck and separate unblock](../obsidian-tasks/web-research-31-user-resolution.md) | medium | 04, 27, 30 |
| 32 | [Configuration circuit breaker](../obsidian-tasks/web-research-32-circuit-breaker.md) | medium | 18, 27 |
| 33 | [Protected persistent cache](../obsidian-tasks/web-research-33-protected-cache.md) | medium | 13, S; I |
| 34 | [Scoped access and explicit transfer](../obsidian-tasks/web-research-34-source-transfer.md) | medium | 24, 33, G |
| 35 | [Exact verification reuse](../obsidian-tasks/web-research-35-verification-reuse.md) | low | 16, 27, 33 |
| 36 | [Format-adapter feasibility contracts](../obsidian-tasks/web-research-36-format-feasibility.md) | medium | 01 |
| 37 | [Isolated text-PDF research](../obsidian-tasks/web-research-37-text-pdf.md) | medium | 16, 36; I/PDF contract |
| 38 | [Constrained rendered pages](../obsidian-tasks/web-research-38-rendered-pages.md) | medium | 10, 16, 18, 36; I/renderer contract |
| 39 | [Visual and scanned sources](../obsidian-tasks/web-research-39-visual-sources.md) | medium | 04, 16, 36; I/visual contract |
| 40 | [Third-party resource revocation](../obsidian-tasks/web-research-40-subresource-revocation.md) | medium | 28, 29, 35, 38, 39 |
| 41 | [Aggregate restricted-mode eligibility](../obsidian-tasks/web-research-41-restricted-mode.md) | medium | 01, 02, 19, 22, 25, 29, 31, 32; I |
| 42 | [Release regression corpus](../obsidian-tasks/web-research-42-release-regressions.md) | low | 07, 08, 09, 12, 14, 15, 16, 17, 26 |
| 43 | [Actual boundary proofs](../obsidian-tasks/web-research-43-boundary-proofs.md) | medium | 10, 25, 33, 34, 37, 38, 39, 40, 41 |
| 44 | [Lifecycle/revocation regressions](../obsidian-tasks/web-research-44-lifecycle-regressions.md) | low | 22, 24, 28, 29, 31, 40 |
| 45 | [Consented live evaluation](../obsidian-tasks/web-research-45-consented-evaluation.md) | medium | 19, 23, 32, 35, 37, 38, 39, 40, 41, 42, 43, 44; separate live consent |
| 46 | [Evidence-backed readiness/docs](../obsidian-tasks/web-research-46-readiness-report.md) | low | 42, 43, 44, 45 |

## Interview decision coverage

The primary implementation owners below must provide their own proof; 42–46 add
regression/evaluation/reporting and cannot excuse missing tests in an earlier task.
The wording in the source specification remains authoritative.

| Decision | Primary tickets |
| --- | --- |
| Q1 | 06, 24, 25 |
| Q2 | 05, 12, 34 |
| Q3 | 10, 11, 38 |
| Q4 | 15, 17, 42 |
| Q5 | 36, 37, 38, 39, 45 |
| Q6 | 26, 27, 28, 40 |
| Q7 | 05, 10, 12, 18 |
| Q8 | 02, 15, 31 |
| Q9 | 02, 03, 04, 19, 39 |
| Q10 | 36, 38 |
| Q11 | 36, 37, 38, 39, 41 |
| Q12 | 17, 18, 23, 45 |
| Q13 | 01, 02, 11, 33, 41, 43 |
| Q14 | 24, 25, 29, 40, 44 |
| Q15 | 06, 08, 15, 42 |
| Q16 | 04, 36, 39 |
| Q17 | 06, 11, 12, 20, 21 |
| Q18 | 09, 11, 13, 14 |
| Q19 | 13, 14, 16, 28, 35 |
| Q20 | 13, 22, 27, 33, 35 |
| Q21 | 21, 29, 44 |
| Q22 | 18, 22, 33, 44 |
| Q23 | 08, 19, 45 |
| Q24 | 06, 08, 20, 21, 23 |
| Q25 | 02, 03, 11, 24, 25, 41 |
| Q26 | 01, 11, 41 |
| Q27 | 07, 26, 27 |
| Q28 | 07, 17, 26, 27 |
| Q29 | 09, 15, 16, 35, 37, 39 |
| Q30 | 36, 38, 40 |
| Q31 | 12, 17, 45 |
| Q32 | 05, 25 |
| Q33 | 32, 45 |
| Q34 | 24, 28, 29, 40, 44 |
| Q35 | 30, 33, 34 |
| Q36 | 20, 26, 30, 31 |
| Q37 | 18, 19, 22, 32, 45 |
| Q38 | 04, 45 |
| Q39 | 01, 36, 37, 38, 39, 41, 43 |
| Q40 | 13, 33, 34 |
| Q41 | 04, 05, 12, 34 |
| Q42 | 42, 43, 44, 45, 46 |
| Q43 | 30, 31, 44 |

## Behavioral proof coverage

These mappings point to real behavior tickets and independent proof work. A listed
number is planned ownership, not a claim that its tests or implementation exist.

| Scenario | Implementation and proof tickets |
| --- | --- |
| T01 | 02, 03, 04, 39, 45 |
| T02 | 02, 11, 24, 25, 41 |
| T03 | 06, 08, 42 |
| T04 | 04, 06, 07, 08, 26, 42 |
| T05 | 07, 26, 42 |
| T06 | 06, 15, 17, 23, 42 |
| T07 | 09, 13, 14, 15, 42 |
| T08 | 16, 37, 39, 42 |
| T09 | 11, 37, 38, 39, 45 |
| T10 | 11, 15, 17, 42, 45 |
| T11 | 10, 38, 43 |
| T12 | 10, 37, 38, 39, 41, 43 |
| T13 | 01, 02, 11, 33, 38, 41, 43 |
| T14 | 07, 11, 12, 20, 30, 42 |
| T15 | 04, 05, 10, 12, 34, 43 |
| T16 | 05, 10, 12, 43 |
| T17 | 27, 43 |
| T18 | 17, 26, 27, 42 |
| T19 | 27, 28, 29, 35, 40, 44 |
| T20 | 24, 25, 29, 40, 44 |
| T21 | 31, 32, 44 |
| T22 | 30, 31, 44 |
| T23 | 13, 14, 16, 28, 35, 40, 42 |
| T24 | 22, 30, 33, 35, 43 |
| T25 | 13, 34, 43 |
| T26 | 08, 20, 21, 23, 44 |
| T27 | 08, 18, 22, 33, 44 |
| T28 | 18, 19, 22, 32, 45 |
| T29 | 17, 18, 23, 42, 45 |
| T30 | 01, 36, 37, 38, 39, 41, 43 |
| T31 | 25, 29, 41, 43 |
| T32 | 40, 43, 44 |

## Frontier and delivery limits

- **Startable now:** 01 (read-only evidence inventory) and 02 (narrow fail-closed
  migration), when execution is separately requested. Creating this backlog starts
  neither task. 03 and 36 become candidates only after their respective blockers.
- **No silent prerequisites:** I/H and missing shared P/E/G/S/R capabilities remain
  visible blockers. A model may not declare them solved by an interface name,
  role called read-only, expired cache, passed detector or user allow-all.
- **Parent unchanged:** no edits or closure of web-tools or existing shared security,
  storage, routing and isolation tasks are part of this authoring change.
- **Not a delivery claim:** source/spec publication, local commits, fake-model tests
  and a readiness report alone do not prove the full feature. Live acceptance is
  finite-suite evidence with its stated limits, and protected limited mode is not
  full PDF/browser/visual parity.

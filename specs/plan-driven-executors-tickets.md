# Plan-driven executors: proposed ticket breakdown

Status: **draft, not published implementation tasks**. Review granularity and
blocking edges before registry publication. Proposed epic ID:
`plan-driven-interactive-executors`. Design:
[contract](plan-driven-executors.md); design ledger:
[plan-agent-contract-design](../obsidian-tasks/plan-agent-contract-design.md).

## Epic outcome

A parent assigns bounded work with its effective context; the child proposes a local
plan; the parent approves the approach; the child executes; the parent accepts or
returns evidence-backed findings in the same retained session. Human authority,
actual-exit resource ownership and the existing permission gate remain intact.

All slices include their own regression checks and user-facing documentation for
what they introduce. Later low-capability test/documentation slices extend explicit
matrices; they do not defer foundational correctness. No slice is permission to
implement outside its stated scope. Each can be demonstrated without finishing the
epic; complete feature rollout waits for integrated evaluation.

Model levels denote capability, not reasoning effort. Distribution: **5 high,
6 medium, 4 low**. Lower-capability work receives approved behavior and concrete
fixtures first; ambiguous architecture returns to its dependency owner. The quality
bar and parent acceptance do not vary with model level.

## Proposed registry tasks

### 01 — Launch a read-only child from an effective-context snapshot

**ID:** `executor-context-snapshot`; **model_level:** high.
**Blocked by:** none; use the landed child runtime and compaction interfaces.
**Why high:** provider-valid history, snapshot ownership and authority separation
require cross-module decisions, not transcription.
**Inputs:** contract D1–D4; existing provider microprojection, macro-compacted
history, retained-child startup, permission and editable-observation rules.
**Scope/output:** one opt-in read-only context launch from parent to retained child;
snapshot metadata and actionable launch errors visible to the parent. Define the
coherent cutoff representation and child identity projection. Existing unbound
launches remain compatible; no local-plan approval or editing yet.
**Acceptance:** full effective context and read-only overall plan arrive; unresolved
tool exchanges do not; the child cannot use parent grants or mutate parent state;
failure leaves no runnable orphan; a fitting snapshot is not shortened for economy.
**Verification:** controlled-provider scenario with macro summary, frozen microstubs,
pending spawn round and fresh child observations; inject snapshot/startup failure;
assert independent parent/child policies and histories.
**Escalation:** if current provider projection cannot be reused safely, stop and
propose a scoped deepening; do not build a second compaction engine.

### 02 — Have the parent approve a child's local plan before execution

**ID:** `executor-local-plan-review`; **model_level:** high.
**Blocked by:** 01 and existing `plan-v2-strong-model-review` plus
`plan-v2-contract-migration-integration`, which validate the plan transitions this
slice extends. These are explicit current safety gates, not the whole Plan v2 epic.
**Why high:** approver identity, preparation policy and version fencing are authority
semantics. Resolve these once before medium/low integration work.
**Inputs:** D1–D6; reviewed Plan v2 lifecycle; 01's immutable snapshot contract.
**Scope/output:** assign -> child proposes local plan -> parent returns findings or
approves -> child executes a bounded operation. Finalize the routed interaction
schema, semantic revision projection, local owner/approver records and preparation
allowlist. Define later clarification/rework operations in that same interface.
**Acceptance:** no edits before approval; approved child execution still passes the
permission gate; wrong parent, stale revision and fake textual approval rejected;
returned plans stay in the same session; no inference slot held while awaiting
review; progress-only updates do not revoke approval, material changes do.
**Verification:** public runtime scenarios for proposal/return/approval and gate
denial after approval; wrong-owner/revision cases; deterministic cancellation/admission
barriers; unchanged ordinary user-owned plans.
**Escalation:** unresolved Plan v2 review blocks landing; a need for broader user
permissions is an Ask, never an approver-role workaround.

### 03 — Present an artifact-bound result for separate parent acceptance

**ID:** `executor-result-acceptance`; **model_level:** high.
**Blocked by:** 02.
**Why high:** durable acceptance and artifact changes require a precise transaction
and evidence identity, not trusting a child summary.
**Inputs:** D6–D7, D9; 02's identities/revisions; existing plan evidence and outcome
receipt paths, owned-worktree conventions.
**Scope/output:** completed execution presents a candidate; parent accepts or returns
findings against the current contract and checked artifacts. Finalize manifest,
durable result/review records and atomic acceptance checks. Include an explicit
same-session next-attempt contract for 04; no separate success scheduler.
**Acceptance:** job completion and child local-plan completion never close the parent
step; changed dirty/untracked/deleted artifacts invalidate stale evidence; duplicate
or stale decisions cannot complete it twice; unresolved findings remain visible.
**Verification:** accept/reject through the public runtime and plan; commit and dirty
manifest fixtures; artifact mutation between check and acceptance; duplicate outcome,
stale contract and receipt recovery fault injection.
**Escalation:** no safe atomic decision or artifact identity means stop/design review,
not a 'verified' flag or broad whole-workspace hash.

### 04 — Return findings and clarification to the same retained executor

**ID:** `executor-linked-rework`; **model_level:** medium.
**Blocked by:** 03.
**Why medium:** owner/revision/interaction decisions are fixed by 02–03; this is a
bounded path through the existing Controller queue and Manager admission.
**Inputs:** D4–D6; approved routed schema, outcome kinds, rework record and tests from
02–03; existing human follow-up and wait/push dedup behavior.
**Scope/output:** parent returns a concrete finding or answers a child clarification;
the same retained session starts one immutable linked attempt with explicit timeout
and current contract/context delta. Do not invent message or plan semantics.
**Acceptance:** child history is retained; new JobID links predecessor; repeated
requests do not duplicate runs; ordinary follow-up queues instead of interrupting;
material rework requires a new local-plan approval; late old outcomes stay stale.
**Verification:** two rework cycles and clarification via public operations; duplicate
send, busy queue, full admission, missing child and timeout cases; parent receipt
contains the final current result once.
**Escalation:** missing transition or schema requirement goes back to 02/03's owner;
no ad hoc direct Engine invocation or silent runtime replacement.

### 05 — Reconcile human intervention and enforce human-stop protection

**ID:** `executor-human-authority`; **model_level:** high.
**Blocked by:** 04.
**Why high:** human/parent races and resume authority cross execution, queueing and
acceptance; mistakes can undo explicit human intent.
**Inputs:** D5–D9; existing origin/Ask/session fences; 04's routed interactions.
**Scope/output:** human clarification, scope conflict and explicit stop reach the
parent as concrete bounded events. Human stop latches parent-resume refusal until
explicit human resumption. Establish a typed projection for later UI work.
**Acceptance:** viewing is not intervention; parent messages cannot answer human Ask;
queued stale parent direction cannot overwrite newer human intent; unresolved
intervention blocks acceptance; stop completion still waits for actual exit.
**Verification:** human-stop followed by parent send; explicit human resume;
parent-stop versus human-stop; simultaneous input/approval/result with controlled
barriers; cross-session Ask and event routing tests.
**Escalation:** uncertainty about who authorized a transition fails closed and is
reported; do not infer authority from transcript text.

### 06 — Request independent review without inheriting parent conclusions

**ID:** `executor-independent-review`; **model_level:** medium.
**Blocked by:** 03.
**Why medium:** snapshot and authority mechanics already exist; add one explicit
selection mode with a testable source manifest, not a new agent lifecycle.
**Inputs:** D4, D7, D10; 01's snapshot interface and 03's artifact/evidence envelope.
**Scope/output:** parent launches a read-only review with requirements and artifacts,
sees the selected/excluded source manifest, and receives findings tied to those
artifacts. Reuse local-plan review; no new nested reviewer agent.
**Acceptance:** parent conclusions excluded by default; original user constraints
preserved; content not read is not reported as reviewed; review cannot self-accept
the parent step; mode difference is explicit.
**Verification:** fixture containing a planted incorrect parent conclusion and a
contradicting artifact; assert actual provider input and result identity, not model
obedience alone; gate and artifact-access denial cases.
**Escalation:** source provenance cannot distinguish requirements from conclusions
-> explicit selection/review request, never claim automatic independence.

### 07 — Resolve a smaller child's context overflow without silent loss

**ID:** `executor-context-budget`; **model_level:** medium.
**Blocked by:** 01.
**Why medium:** budget policy is fixed; reuse the existing compaction mechanism and
snapshot interface to expose a bounded choice-and-retry flow.
**Inputs:** D3–D4; 01's cutoff and size metadata; current model ceiling/compaction
behavior. Preserve human model-selection control.
**Scope/output:** too-large launch reports required/available budget and supports an
explicit retry with a suitable model, narrower assignment or authorized child-local
compaction. Record changed representation; no new summarizer.
**Acceptance:** reserve system/tools/plan/output budget; no silent prompt-only
fallback; parent context unchanged; requirements and current approval obligations
survive allowed compaction or launch fails visibly; explicit rebase reconciles IDs.
**Verification:** fitting, exact-boundary and overflowing controlled-model cases;
rejected compaction and unavailable model; compare parent state before/after;
retry yields either a valid complete input or actionable failure.
**Escalation:** inability to preserve required material blocks launch; compactor
architecture changes return to 01's owner.

### 08 — Recover assignment relationships without resurrecting dead work

**ID:** `executor-assignment-recovery`; **model_level:** high.
**Blocked by:** 05, existing `multisession-lifecycle-restore` and
`job-recovery-process-ownership`.
**Why high:** durable ownership, stop latches and actual-process liveness must agree
across restarts; this cannot be delegated as a serialization-only change.
**Inputs:** D8–D9; 02/03 durable schemas; 05 stop/intervention states; existing restore
and process-ownership results.
**Scope/output:** restore the assignment/plan-review/result-review relationships
when inspecting or explicitly resuming a retained history. Add executor integration
to existing close/restore, not duplicate close UI or a second history index.
**Acceptance:** corrupt/unknown records preserved and reported; no invented running
or accepted state; human-stop protection persists; explicit resume gets a new
attempt and validates current authority; close does not delete history.
**Verification:** crash/restart at proposal, approval, result receipt and acknowledgement;
live foreign-process ownership; closed View not reopened by synchronization;
corrupt records and missing worktree.
**Escalation:** blocked lifecycle/process ownership prerequisites stay blocked; do
not downgrade them to a best-effort process kill or fabricated success.

### 09 — Inspect executor review state and act on the correct session

**ID:** `executor-review-ui`; **model_level:** medium.
**Blocked by:** 05.
**Why medium:** transitions and authenticated projection are fixed; this wires
existing session selection and actions through them.
**Inputs:** D5–D8; 05's typed projection; existing retained View/selector controls.
**Scope/output:** user can inspect assignment and local-plan review state, see
pending result acceptance, reach the correct transcript and explicitly resume a
human-stopped executor. Reuse existing controls; broader sidebar/notification
redesign stays with `multisession-background-attention`.
**Acceptance:** job done differs from accepted; actions target displayed origin;
no focus theft; busy/stopped/conflicted states reflect runtime truth; UI does not
perform its own approval checks or change a sibling's model.
**Verification:** public input-to-controller-to-render tests for two children,
background outcome, review/rework and explicit human resume; keyboard/mouse checks
using existing navigation rather than new sidebar requirements.
**Escalation:** missing runtime transition goes back to 05; do not implement it in
a widget. No dependency on full sidebar delivery is needed.

### 10 — Add exact status explanations to existing executor views

**ID:** `executor-status-explanations`; **model_level:** low.
**Blocked by:** 09.
**Why low:** one approved finite projection-to-text mapping; no lifecycle decisions.
**Inputs:** 09's rendered view and fixtures; D8 state/action table. Freeze the exact
labels and help copy during 09's parent acceptance.
**Scope/output:** wire approved concise explanations for waiting for parent plan
review, executing, result awaiting review, changes requested, human-stopped and
accepted into the existing selected executor view and help affordance.
**Acceptance:** every approved projection has its specified explanation; job completion
never says accepted; unknown state uses the approved neutral fallback; narrow views
remain readable. No new state, event or action.
**Verification:** extend the existing renderer table tests with approved expected
strings and narrow/wide fixtures; manually inspect one selected executor view.
**Escalation:** missing state/copy or layout redesign needed -> return to 09 owner;
never guess a transition or rewrite the selector.

### 11 — Teach parent and child the approved review-and-rework protocol

**ID:** `executor-prompt-skills`; **model_level:** medium.
**Blocked by:** 05 and 06.
**Why medium:** several roles need consistent guidance, but runtime semantics and
public operations are already fixed. This delivers the actual model-facing flow,
not speculative policy text.
**Inputs:** D10; finalized schemas and denial messages; accepted scenarios from
02–06; project agent-writing guidelines.
**Scope/output:** permanent identity/authority rules plus selectable parent,
executor and independent-review skill workflows, wired into real prompt assembly.
Include capability-specific brief and escalation examples and context update usage.
**Acceptance:** child proposes plan and waits for parent review; parent distinguishes
approach approval from result acceptance; parent messages never impersonate user
permission; skills cannot restore disabled capabilities or bypass runtime guards.
**Verification:** prompt assembly tests for each role/mode and selected skills;
scripted propose/return/approve/rework walkthrough; adversarial quoted approval and
missing-context examples produce the required explicit instructions.
**Escalation:** unclear semantics go to foundation owners; prompt-only security
patches and adding new tools through prose are forbidden.

### 12 — Extend the deterministic local-plan and acceptance rejection matrix

**ID:** `executor-review-negative-matrix`; **model_level:** low.
**Blocked by:** 03.
**Why low:** test-only expansion of public fixtures with prescribed inputs/results;
foundation slices already cover correctness and supply the harness.
**Inputs:** 02/03 public test helpers and approved expected cases: wrong parent,
stale local-plan revision, changed assignment, duplicate approval, duplicate result,
result without approval, completed job without parent acceptance.
**Scope/output:** table-driven tests for those exact cases; no production changes.
**Acceptance:** each case asserts visible rejection/idempotent response and unchanged
parent step; tests use public operations and deterministic fixtures, not internals.
**Verification:** run the narrow added tests, then existing related suites; verify
one deliberate local expectation mutation fails and restore it before committing.
**Escalation:** failure reveals a production defect or missing fixture -> report
reproduction and return to 02/03 owner; do not repair authority code in this task.

### 13 — Extend the fixed context-budget regression matrix

**ID:** `executor-budget-regressions`; **model_level:** low.
**Blocked by:** 07.
**Why low:** deterministic fixture additions against a settled budget interface.
**Inputs:** 07's public fixtures and fixed expected budget accounting; cases for
exact fit, one-token overflow, output reserve, rejected compaction and unchanged
parent projection.
**Scope/output:** test-only matrix covering those cases plus the specified too-large
user message. No compaction, token estimation or provider behavior changes.
**Acceptance:** no case silently launches prompt-only; fitting input is preserved;
parent snapshot identity/content unchanged after child retry.
**Verification:** targeted matrix and existing context regression suite; verify a
local expectation mutation fails and restore it before committing.
**Escalation:** tokenizer nondeterminism or production defect -> report to 07 owner;
never relax expected preservation to make a test green.

### 14 — Publish a verified user walkthrough of local-plan review and rework

**ID:** `executor-user-walkthrough`; **model_level:** low.
**Blocked by:** 09 and 11.
**Why low:** documentation from a fixed, observed workflow and existing terminology.
**Inputs:** accepted public operations, 09 screenshots/transcript fixtures and 11
role walkthrough; D8 lifecycle distinctions.
**Scope/output:** runnable documented example: assign, inspect proposed plan, parent
approval, result review, return finding, human stop and explicit resume. Link existing
history/close documentation without promising unfinished recovery behavior.
**Acceptance:** commands/actions match the build; permission approval distinct;
completed child not called accepted or closed; capability and effort not conflated.
**Verification:** replay against a controlled provider/session, compare actual output
with the example, check links and CHANGELOG entry for published behavior.
**Escalation:** behavior differs from the accepted fixture -> report it, do not invent
commands or quietly document an unreviewed alternative.

### 15 — Evaluate the complete workflow across model capabilities

**ID:** `executor-quality-evaluation`; **model_level:** medium.
**Blocked by:** 06, 07, 08, 10, 11, 12, 13 and 14.
**Why medium:** run an agreed scenario matrix and investigate bounded discrepancies;
new architecture and policy choices are escalations, not part of evaluation.
**Inputs:** full contract, accepted slices and public test harnesses; fixed tasks
with human-reviewed expected requirements, artifacts and rejection cases.
**Scope/output:** repeatable integration scenarios and bounded high/medium/low model
trials with an evidence report. Measure false acceptance, lost requirements,
rework, authority violations and unresolved contradictions before timing/cost.
**Acceptance:** scripted invariants pass; failures recorded without cherry-picking;
real-model trials use comparable tasks and disclosed context modes; no claim that
passing samples proves general quality; parent/human reviews conclusions before rollout.
**Verification:** assign -> local plan returned -> approved -> execute -> result
returned -> reapprove -> accepted; human conflict/stop, restore, budget mismatch and
independent counterexample review; targeted races and documented manual interaction.
**Escalation:** any false acceptance/authority breach blocks rollout and becomes an
issue for the owning slice; unclear expected quality criteria go to the user rather
than being replaced with a speed target.

## Dependency review

New-task edges (prerequisites on the right):

- 01: none
- 02: 01; existing Plan v2 review and migration/integration
- 03: 02
- 04: 03
- 05: 04
- 06: 03
- 07: 01
- 08: 05; existing lifecycle/restore and process ownership
- 09: 05
- 10: 09
- 11: 05, 06
- 12: 03
- 13: 07
- 14: 09, 11
- 15: 06, 07, 08, 10, 11, 12, 13, 14

Existing blockers retain their own owners/status. Proposed tickets are not marked
ready while any required edge is unresolved. No ticket takes over the broader
session panel, generic security backlog, or existing Plan v2 review. During registry
publication use stable IDs, per-task acceptance/verification and explicit blocker
text; do not assume the registry automatically schedules textual dependencies.

## Review requested

1. Is this granularity appropriate, particularly the three initial foundation slices?
2. Do the listed existing-task gates match the capabilities actually needed?
3. Should any slices be merged or split before they become registry tasks?

The primary test seam remains the existing Controller/interactive runtime, with
real plan/admission/receipt paths and controlled external execution. Approval of this
breakdown includes this proposed testing approach; no new runtime is implemented here.

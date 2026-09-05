# Plan-driven interactive executors

Status: approved design and testing seam; implementation tickets published, runtime not implemented.
Design task: [plan-agent-contract-design](../obsidian-tasks/plan-agent-contract-design.md).
Approved work: [ticket breakdown](plan-driven-executors-tickets.md).
Epic: [plan-driven-interactive-executors](../obsidian-tasks/plan-driven-interactive-executors.md).
Evidence baseline: CozyPhi main `753c017`, inspected 2026-09-06.

## Problem Statement

A parent can already launch an isolated interactive child, inspect its retained
conversation, and receive linked outcomes. That does not yet express a bounded
piece of an approved plan, transfer the parent's effective working context, let
the parent approve the child's approach, or distinguish execution from acceptance.
A short hand-written brief loses decisions; indiscriminate transcript copying
risks transferring the parent's identity and authority. Starting a new child for
every correction loses useful task-local history.

Quality is the objective. Parent review may take as long as needed. Parallelism,
latency and token savings do not justify weaker acceptance or missing context.

## Solution

The parent owns the overall plan and gives a retained child an explicit assignment.
The child receives a full effective-context snapshot and the whole read-only parent
plan, prepares its own local execution plan, and submits it to the parent. The
parent approves that approach or returns findings. Execution and verification
follow; the parent then separately accepts the result or requests rework in the
same child session.

Three distinct contracts remain visible:

| Contract | Owner | Approval and effect |
| --- | --- | --- |
| Overall plan | Parent, under user control | Existing user/harness plan rules |
| Bounded assignment | Parent | Defines scope, output, constraints and acceptance |
| Local execution plan | Child | Parent approves a particular revision within the assignment |

Parent approval is a limited harness-recognized decision, **not user permission**.
A child cannot approve its own execution plan or complete the parent step.

## User Stories

1. As a parent, I want to assign a bounded part of an approved step without rewriting all relevant context.
2. As a child, I want the complete effective parent context and read-only overall plan so that I understand decisions and dependencies.
3. As a child, I want my own execution plan so that I can organize investigation, changes and verification.
4. As a parent, I want to review and return that plan before execution so that a mistaken approach is corrected early.
5. As a child, I want actionable approval or findings tied to the exact plan I proposed.
6. As a parent, I want a corrected plan in the same session rather than a fresh child without its working history.
7. As a user, I want agent approval never to impersonate my permission to run a protected action.
8. As a child, I want clarification and scope-change requests to reach the parent without pretending to be human Ask answers.
9. As a parent, I want to inspect outcomes, artifacts and verification before accepting work.
10. As a parent, I want rework to produce a new linked attempt and preserve prior evidence without treating it as current proof.
11. As a user, I want my intervention to be visible and ordered without silently rewriting the parent plan.
12. As a user, I want my explicit stop to prevent an automatic parent restart.
13. As a user, I want interrupt, stop, close and history inspection to be different actions.
14. As a parent, I want stale or duplicate outcomes never to satisfy a newer contract.
15. As a child using a smaller model, I want an explicit budget decision rather than silently missing requirements.
16. As a parent, I want independent review to receive requirements and evidence without being steered by my conclusions.
17. As a user, I want medium and low models to receive precise executable slices without a lower quality bar.
18. As a parent, I want failures and blocked work reported with a recovery action, not disguised as completion.
19. As a user, I want restart recovery to preserve history without inventing live processes or accepted results.
20. As a maintainer, I want one execution/admission mechanism and tests through existing public seams.

## Implementation Decisions

### D1. Reuse owners, not another scheduler

Use the existing Controller queue, Manager admission, retained Views, App scheduling,
plan module and compaction. Extend their interfaces at the existing seams. Do not
add a general DAG/pipeline runner, shared mutable conversation, or nested delegation.
The logical records below do not require one new package each.

The child plan uses the existing plan representation and transition machinery with
an explicit local owner and approver role. The parent plan snapshot is contextual
input, never the target of the child's plan mutations. The harness resolves owners;
model-supplied IDs do not grant access. Runtime, tool and UI callers share the same
transition checks rather than duplicating them.

### D2. Identity and versioned assignment

An assignment binds the owner/process scope, parent conversation, parent plan and
stable step ID, child session, assignment contract revision and context snapshot ID.
Each execution attempt has an immutable JobID and links its predecessor. A retained
child session may serve successive attempts or different steps; session ID is not
an assignment identity.

The contract includes goal, allowed changes, exclusions, deliverable, acceptance,
verification, worktree/artifact location, capability choice and escalation conditions.
Record selected model capability separately from reasoning effort; model selection
remains human-controlled under existing policy.

Use a semantic contract revision or digest, distinct from the plan store's general
audit revision. Changing unrelated progress must not invalidate every child. Changing
the assigned goal, allowed scope, constraints, verification or acceptance must.
An overall plan reapproval requirement still applies before dispatch or acceptance;
semantic matching is not a bypass of its gate.

### D3. Effective context is a snapshot, not a copied authority

Default executor input is the full effective working context: macro summary,
retained history, the parent's frozen microprojection, applicable instructions,
working decisions and the full read-only plan. Building durable session history
alone is insufficient to reproduce the provider projection.

Capture a coherent immutable snapshot at a legal conversation boundary. When spawn
occurs in a tool round, inherited history ends at the last complete provider-valid
prefix; the harness separately includes the current assignment and authorized
completed-round facts. It must not copy an unresolved tool invocation as a runnable
child action, fabricate its result, or include half of a sibling tool exchange.
Record the cutoff and any deferred current-round material explicitly. The child
must wait or request an update if deferred material is required to proceed.

Rebuild the child's active system identity, tool inventory and policy independently.
Inherited parent instructions and messages retain source/trust labels; parent
system text is not installed as the child's system authority. The child sees that
it is the executor, not the parent whose history it is reading. Do not serialize
permission decisions, active Ask requests, edit capabilities, session credentials,
or tool availability as transferable grants. History may mention old approvals or
anchors, but those have no operative capability in the child. Existing input-policy
and secret-handling rules still apply; full context is not a new export exemption.

Snapshot publication/admission is all-or-nothing: failure leaves no runnable orphan.
Authorized large artifacts may be referenced durably, but a reference does not count
as content the child has read. Snapshot and child history remain separately owned.
Shared understanding does not imply shared mutable files: the contract identifies
owned worktree and paths; observations must be refreshed against that checkout.

### D4. Budgets, updates and independent review

Preflight against the actual child model's context ceiling, reserving room for its
instructions, tools, plan and output. A snapshot that fits is not shortened merely
for economy. If it does not fit, report the mismatch and offer a capable model,
a narrower assignment, or an explicitly approved child-local compaction using the
existing algorithms. Record what representation changed. Never silently fall back
to only a prompt, truncate requirements, or mutate the parent's projection.

Follow-ups retain the child's history. Send explicit versioned context/contract
deltas rather than appending a full parent snapshot on every exchange. A new full
snapshot is an explicit rebase with a new ID and reconciliation, not silent replacement.
Child compaction must preserve assignment identity, current contract, approval,
open findings and acceptance obligations, and reload applicable skills.

Independent review is a named alternative context mode: requirements, original
constraints, artifacts and verification evidence, with parent conclusions excluded
by default. It is not an executor silently receiving less context. Record selected
sources and exclusions; keep original evidence available for authorized inspection.
Do not claim independence merely because another model read the same conclusions.

### D5. Local plan approval before execution

The local plan progresses through draft, submitted, approved and superseded states
using the existing plan's corresponding lifecycle representation. These are
logical states, not a requirement to add duplicate persisted enums.

The parent decision binds assignment revision, local plan identity, semantic plan
revision and context snapshot. Only the owning parent can approve through an
explicit harness operation. Child prose, tool results and quoted human text cannot
supply approval. A rejected plan remains in the same session with specific findings.

Before approval, a child can propose its plan and request clarification. Any
read-only investigation needed to prepare it must be explicitly allowed by the
assignment's preparation scope. Tool gate permission alone does not imply execution
plan approval. No edits or execution side effects are permitted as 'preparation'.
The parent can allow a minimal one-step plan for trivial work; there is no implicit
unreviewed execution shortcut in this protocol.

Changes to approach, action classes, scope, verification, step order/dependencies
or relevant context invalidate approval and require renewed parent review.
Progress/evidence bookkeeping and nonsemantic wording corrections do not. Parent
approval cannot broaden the assignment; the parent must revise that contract first.
Unknown classifications fail closed to review rather than guessing materiality.

Pending plan review releases the active inference/job resources on actual exit,
while retaining the View and local plan. Parent feedback starts a new linked attempt
through existing admission. Do not keep a running worker waiting indefinitely or
consume a second scheduler slot for a review state.

### D6. Follow-up, clarification and rework

Provide one routed parent-to-retained-child interaction interface, supporting plan
feedback/approval, clarification, explicit context update and result rework. Exact
tool naming and schema are settled before dependent implementation slices begin.
Each request includes the expected child/assignment identity and current revisions,
origin, sequence/dedup key and intended operation. Permission and plan gates still
run before dispatch; repeated sends cannot create duplicate work.

Normal messages use the Controller queue at safe turn boundaries. Interruption is
a separate operation, never an accidental side effect of a follow-up. Rework starts
a fresh linked JobID in the same child, with new findings and explicit timeout/model
policy; it does not reset permissions or inherit a previous run's approval blindly.
Clarification from the parent is task direction, not a human permission response.

Result, clarification and local-plan proposals are distinct durable outcome kinds.
The parent sees the requested decision and evidence needed to make it. The result
path retains outcome -> parent receipt -> source acknowledgement ordering and
wait/push dedup. Delivery is not acceptance and no exactly-once cross-crash execution
claim is introduced.

### D7. Parent acceptance is a separate checked transition

An outcome identifies contract and local-plan revisions, attempt, context snapshot,
artifact identity, executed checks and their results, deviations and unresolved risks.
A result is a candidate for acceptance, not proof because it says 'done'. Child local
plan completion, command exit zero, job terminal state and result delivery cannot
complete the parent step.

Acceptance records an explicit parent decision in the parent plan/evidence flow;
there is no separate success scheduler. Check current contract/plan approval,
assignment identity, unresolved interventions and artifact identity atomically at
the decision point. Bind evidence to the checked commit or to an immutable manifest
of owned dirty/untracked artifacts, including deletions. Changes after verification
invalidate reuse for that artifact; a later independent edit is not retroactively
covered by acceptance. Do not hash unrelated workspace files or expose secrets in
manifests. Exact manifest representation is a prerequisite of the acceptance slice.

Rejection carries specific findings and expected verification, not 'try harder'.
Parent review ends when criteria are satisfied, required checks are examined,
relevant risks are addressed and contradictions resolved, not when a time budget
expires. Parents may explicitly justify reusing unchanged evidence; the harness
must never infer that justification from a prior successful run.

### D8. Human intervention and lifecycle

Human and parent messages have distinct authenticated origins and ordering.
Viewing a transcript is not intervention. User clarification or edits that may
affect scope, approach or evidence mark the assignment as needing reconciliation;
they do not silently mutate the parent plan. Notify the parent with the relevant
bounded change and conflict, respecting source/access policy. Do not treat an
uninterpreted 'user_intervened' flag alone as sufficient reconciliation.

The parent must resolve conflicts with the user's direction; it cannot overwrite
the user by sending an older contract again. The UI shows execution, local-plan
review and result acceptance separately.

| Action/state | Meaning |
| --- | --- |
| Interrupt turn | End the current inference/tool turn under existing cancellation rules; not acceptance or View disposal |
| Stop assignment | Request termination, preserve history, wait for actual exit before capacity release |
| Human stop | Additionally latch no automatic parent resume; only explicit human resume/new authorization clears it |
| Close View/runtime | Deliberate stop handling for active work, dispose after exit, remove registry ownership; preserve disk history |
| Inspect history | Read retained history; do not create a live job |
| Resume history | Explicit new runtime/attempt, revalidate contract, approvals and ownership; not resurrection of the old process |

A parent-requested stop does not masquerade as a human stop. A completed job is not
a closed View; a timeout limits the attempt, not retained tab lifetime. Late events
from stopped/replaced assignments cannot restore them or accept their work. Reuse
existing lifecycle/restore and process-ownership backlog rather than duplicating it.

### D9. Failure, recovery and compatibility

| Situation | Required behavior |
| --- | --- |
| Snapshot too large or unavailable | No partial launch; actionable alternatives and retained parent state |
| Approval belongs to another owner/revision | Reject without starting execution |
| Approval/result arrives twice | Stable recorded decision; no second run or step completion |
| Contract changes during execution | Mark mismatch, stop or reconcile explicitly; preserve useful evidence as stale |
| Human input races queued parent feedback | Ordered conflict, no silent overwrite or automatic acceptance |
| Stop returns before process exits | Keep resources owned until actual exit |
| Parent unavailable | Persist proposal/result for later review; never self-approve or endlessly auto-retry |
| Crash between outcome and acknowledgement | Reconcile durable receipt; no invented success or exactly-once execution guarantee |
| Child runtime lost | Show unavailable/stopped, preserve history; explicit linked replacement or resume |
| Worktree/artifact changed after checks | Reject stale evidence or require re-verification |
| Compaction changes presentation | Preserve semantic identity/obligations; report material information loss |

Roll out opt-in plan-bound assignments beside existing unbound children. Preserve
headless spawn/wait behavior and ordinary interactive conversations. Migrate durable
records with explicit schema handling; unknown/corrupt records fail visibly and
retain history. Cross-project inheritance and new external backends are not part of
the initial rollout.

### D10. Runtime, prompt and skills

| Layer | Responsibility |
| --- | --- |
| Runtime | Ownership, valid context projection, revision checks, approval/permission separation, safe queueing, stop latch, evidence identity, durable delivery, actual-exit release |
| System prompt | Explain parent/executor identity, read-only parent plan, required local-plan review, source trust, escalation and distinction between execution and acceptance |
| Parent skill | Bound assignments, choose capability, review local plans, resolve findings and independently verify results |
| Executor skill | Draft a feasible local plan, request parent review, remain in scope, report evidence/uncertainty, rework in the same session |
| Independent-review skill | Work from requirements/artifacts, seek counterexamples, distinguish observation from inherited conclusions |

Skills guide methods, not grant powers or enforce security. Prompts are not a
substitute for runtime fences. Child skills are explicitly selected for its work;
inherited knowledge of a parent's skill is not automatic activation of that skill.
Decomposition must provide executable inputs and escalation for medium/low capability;
reasoning effort is a separate setting, never a synonym for capability.

## Testing Decisions

The proposed primary test seam is the existing public Controller/interactive runtime
flow, with a controlled runner/provider and the real plan, admission and receipt
paths. Tests drive assign -> propose -> parent approve -> execute -> present ->
accept/rework and assert observable state/events/artifacts, not private call order.
Existing follow-up, origin/Ask isolation, receipt fencing and model-selection tests
are prior art. Narrow projection/plan tests supplement, not replace, that flow.
UI tests use the real session projection with controlled input; widgets remain dumb.

Required scenarios include invalid tool-round inheritance; fresh anchors; unchanged
parent context after child compaction; wrong-parent and stale-plan approval; no edits
before approval; gate denial after valid parent approval; local plan returned twice;
result rework in the same child; duplicate delivery; human-stop resume refusal; queued
human conflict; actual-exit capacity; corrupt restore records; dirty-artifact changes;
smaller-model overflow; independent review without parent conclusions. Exercise
cancellation and races with deterministic barriers rather than sleeps.

Quality evaluation uses scripted scenarios plus bounded real-model trials across
high/medium/low capability. Record missed requirements, false acceptance, rework and
unresolved contradictions; cost/latency are secondary observations. Repeatability,
fixtures and human-reviewed expected results are required. No quantitative benefit
of full context has yet been demonstrated; it is the agreed default, not an
experimental conclusion.

## Out of Scope

Runtime implementation in this design task; automatic user permission delegation;
child modification of the parent plan; nested agents; general pipeline/DAG tools;
a second scheduler; shared mutable context; automatic history deletion; replacing
the broader session sidebar; claiming complete multisession delivery; priority
optimizations that degrade context or acceptance.

## Further Notes

### Existing backlog and evidence

- [Plan v2](../obsidian-tasks/plan-v2-agent-work-contract.md): reuse lifecycle,
  approval, evidence and skill semantics. Its review/migration work is not assumed
  complete. Gate implementation slices on the specific required reviewed seams,
  not every unrelated item in the epic.
- [Lifecycle/restore](../obsidian-tasks/multisession-lifecycle-restore.md) owns user
  close, active-work confirmation, recent sessions and restoring the open set.
- [Background attention](../obsidian-tasks/multisession-background-attention.md)
  owns the broader grouped panel/focusable notification experience.
- [Process ownership](../obsidian-tasks/job-recovery-process-ownership.md) owns safe
  handling of live processes during job recovery.
- Existing retained-child and session-local model behavior is baseline, not new work
  claimed by this design. `/new` exists; `Registry.Close` was only called by tests
  at the inspected baseline. Global Editor close is not a user single-View close.

Official comparison actually reviewed: Claude Code
[subagents documentation](https://code.claude.com/docs/en/sub-agents.md), retrieved
2026-09-06, sections 'Resume subagents', 'Auto-compaction', 'Fork the current
conversation' and 'Observe and steer running forks'. This is a dated documentation
observation, not a source-code or runtime verification. It describes same-agent
SendMessage resume, human-stop protection, full-context forks and child transcript
interaction. It also describes parent-affecting model commands in child views;
CozyPhi deliberately retains its own session-local model semantics. Identical
prompt-cache reuse and deletion policies are not imported. Downloaded Codex and
OpenCode pages were not sufficiently reviewed and support no comparison claims.
No external task-list behavior is asserted equivalent to CozyPhi Plan v2.

### Approval and implementation decisions still to settle

The user approved the primary testing seam and all 15 slices before publication.
The agreed semantics above are not left to lower-capability implementers to invent.
High-capability foundation slices must settle and document
exact tool schemas, durable record/version migration, semantic revision projection,
artifact manifest and atomic acceptance transaction before dependent slices start.
A blocked prerequisite is reported, not papered over with a prompt rule.

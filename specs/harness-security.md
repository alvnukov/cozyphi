# Opt-in harness security: authority, confidential data and model-assisted detection

Status: design approved for specification and ticket publication on 2026-09-09;
implementation and enforcement readiness are not approved by this document.
[Delivery slices](harness-security-tickets.md) ·
[Planning task](../obsidian-tasks/harness-security-spec.md) ·
[Existing security epic](../obsidian-tasks/harness-security-hardening.md)

## Problem Statement

An agent can turn an instruction hidden in repository content, a tool result or a
web page into an action with the user's authority. It can also send confidential
workspace data to a model or publish it through a tool without that being the
user's intention. A read-only child still has a model recipient. A summary still
contains information derived from its inputs. Permission checks on a command
string do not constrain everything the resulting process can read or transmit.

Existing permission and web controls are useful but are not a general information
flow policy. Users need explicit recipient control, durable provenance and safe
failure behavior without depending on an expensive model for every decision.
They also need an honest master switch: protection must be opt-in, observable,
and distinguishable from storage encryption and ordinary tool permissions.

## Solution

Introduce an opt-in host-owned security policy around material admission, actions,
model dispatch and sensitive persistence. The workspace is confidential by default
when the feature is enabled. Fast, inexpensive models supply detection signals;
they never supply authority. Deterministic policy remains authoritative, with
human decisions only where that policy permits them.

The master switch defaults to off. First enablement enters observe mode: hard
policy applies, while model suspicions are measured rather than enforced. Later,
separately enabled enforcement quarantines suspicious material and pauses when a
required check cannot finish. Existing permission and web protections remain in
all modes. This design does not claim process-level egress prevention in V1.

A user can explicitly trust a local or self-operated model profile to receive
secrets after a concrete warning. Until encrypted storage is ready, only an
isolated trusted guard may receive raw secrets. Once storage is ready, trust can
apply to every role using that exact profile. Already encrypted sessions remain
encrypted and can continue without guards when the master switch is turned off.

## User Stories

1. As a user, I want new security processing off by default, so that an upgrade does not silently inspect or transmit more data.
2. As a user, I want first enablement to use observe mode, so that I can see model signals before accepting model-driven interruptions.
3. As a user, I want hard policy enforced during observation, so that observation is not permission to leak data.
4. As a user, I want existing permissions and web protections preserved while off, so that the switch is not an unrelated bypass.
5. As a user, I want a visible mode and coverage indicator, so that partial rollout is not mistaken for complete protection.
6. As a user, I want only my explicit actions to change security settings, trust and grants, so that content cannot approve itself.
7. As a user, I want confidential workspace data bound to named recipients, so that sending it to one model does not authorize publication.
8. As a user, I want one-use approvals bound to the actual data and action, so that a rewritten command cannot reuse stale consent.
9. As a user, I want an explicit bounded task grant, so that repetitive authorized work need not receive a blanket session exemption.
10. As a user, I want repository instructions, memory and tool text treated as data for security authority, so that forged control messages cannot grant access.
11. As a user, I want provenance retained after compaction and restart, so that old hostile or sensitive input cannot become clean through summarization.
12. As a user, I want child agents and readers covered, so that delegation cannot bypass recipient policy.
13. As a user, I want every actual model dispatch checked, so that retries, fallbacks and compactors do not leak through a secondary path.
14. As a user, I want inexpensive guards to inspect only necessary material, so that detection has bounded exposure and cost.
15. As a user, I want malformed, incomplete or unavailable guard results called unknown, so that failure is not mistaken for safety.
16. As a user, I want explicit confirmation before strong-model escalation, so that cheap detection does not silently become expensive processing.
17. As a user, I want suspicious material quarantined in enforcement, so that an automatic summary cannot launder it into trusted input.
18. As a user, I want to retry or decline a paused action, so that an unavailable checker cannot force an unsafe continuation.
19. As a headless operator, I want execution to stop when required approval cannot be collected, so that missing UI is not consent.
20. As a user, I want an informed local/own-profile trust decision, so that I understand secret access and server-side logging risks.
21. As a user, I want profile trust bound to provider, endpoint and model, so that switching models cannot inherit unrelated trust.
22. As a user, I want limited operation without a trusted profile, so that refusal to trust does not require disabling every useful feature.
23. As a user, I want sensitive session artifacts encrypted, so that protecting the primary history does not leave plaintext summaries or child transcripts.
24. As a user, I want keys in an OS secure store or protected by a separate passphrase, so that encryption is not a key beside the ciphertext.
25. As a user, I want key failures to stop sensitive persistence, so that storage never falls back to plaintext.
26. As a user, I want encrypted sessions to stay encrypted while security is off, so that changing runtime protection does not decrypt my history on disk.
27. As a user, I want a choice before moving sensitive context to an untrusted model, so that redaction is not silently declared safe.
28. As a user, I want safe local audit metadata, so that investigating a decision does not itself expose the underlying secret.
29. As a user, I want latency and false-stop measurements on real task scenarios, so that fragment-level scores do not hide an unusable workflow.
30. As a maintainer, I want attack outcomes measured at controlled recipients and action boundaries, so that a detector's label is not confused with prevention.
31. As a user, I want turning protection off to cancel pending security work without replaying paused actions, so that a mode change does not execute old requests.
32. As a user, I want the process-isolation gap disclosed, so that command approval is not advertised as a network sandbox.

## Implementation Decisions

### D1. Modes and non-model authority

The proposed settings are `security.enabled` (default false) and `security.mode`
(first enablement: observe). They are design names, not existing configuration.
Only a user-controlled, non-model path can change them. Project configuration,
repository instructions, skills, memory, tool output, plans and model-generated
arguments cannot mint consent. Ordinary permission-bypass flags do not turn the
new security layer off. A hard prohibition cannot be bypassed by ordinary approval.

| State | New deterministic policy | Model checks | Model suspicion | New security audit |
|---|---|---|---|---|
| Off | Disabled | No calls or shadow work | No new security ask/quarantine | Disabled |
| Observe | Active | Bounded, permitted inputs only | Nonblocking signal; record potential stop | Safe local metadata |
| Enforce | Active | Required where policy says so | Quarantine/pause for human decision | Safe local metadata |

An unknown detector result in observe is recorded as unknown and does not relax
hard policy. In enforce, a required check that cannot complete pauses execution;
retry or an exception is available only where policy permits. An unknown material
classification is not a clean classification in either enabled mode.

Turning off cancels pending checks and security-specific asks, prevents stale
callbacks from applying decisions, and does not automatically replay suspended
actions. Already dispatched data cannot be recalled. An explicit subsequent user
action may continue with legacy controls. Encrypted persistence remains in force
for an already encrypted session, without running guards or security audit.

Partial slices must disclose coverage and cannot offer complete enforcement.
Headless callers receive structured mode, coverage, reason and required-action
information. With no approved way to obtain necessary consent, they stop.

### D2. Threat model and vocabulary

The protected assets are confidential workspace content, credentials, sensitive
conversation derivatives and user authority. Attackers can control or poison
repository files, instructions, memory, web content, tool descriptions/results,
process output, child summaries and persisted content. They may forge apparent
system messages or approvals, encode payloads, spread instructions over turns,
and avoid overt tool requests while poisoning an answer.

The trusted computing base includes the host policy/consent implementation and
its runtime. A compromised host process, OS, trusted adapter or trusted model
server is outside the V1 prevention claim. Process isolation is a separate stage.

- **Material:** data plus a host-owned identity/version and provenance envelope.
- **Authority:** permission to decide policy; never inferred from material text.
- **Sensitivity:** confidentiality restrictions, independent of instruction trust.
- **Sink:** the actual recipient or publication destination, not just a tool name.
- **Grant:** user-authorized, bounded transfer/action capability under hard policy.
- **Trusted profile:** explicitly accepted local/own provider-endpoint-model tuple;
  not a safety verdict and not authority over tools.
- **Verification state:** unchecked, checked for a defined scope, suspicious or
  unknown; never an unqualified guarantee of safety.
- **Quarantine:** inaccessible-to-normal-consumption material reference awaiting
  a permitted resolution, not an automatically sanitized substitute.
- **Derivation:** host-recorded relationship between inputs and produced material.

Confidential-by-default does not prohibit every intended model use. It requires
an explicit recipient decision or a still-valid scoped grant before disclosure.
Approval for one recipient never authorizes another recipient or public posting.
Adapter-managed authentication credentials can serve their intended protocol
without entering prompts; this is not permission to expose them in model content.

### D3. Deep policy module and host metadata

Introduce one deep `security` module with three conceptual operations: admit
material and return an allowed representation or quarantine reference; assess a
transfer/action and return a decision with conditions; apply an authentic human
decision. Keep policy, provenance, grants, quarantine lifecycle, guard scheduling,
cache and safe audit behind that interface. Cryptography belongs to session
storage rather than to a parallel policy-owned persistence stack.

The host envelope carries source/version, authority class, sensitivity, permitted
recipients, verification scope/state and derivation. It is not reconstructed from
model text and is not embedded as forgeable control syntax. The existing
host-only delivery-identifier pattern is useful prior art, but its field must not
be repurposed for security metadata.

Ingress includes user-designated sensitive input; project instructions, skills
and memory; file read/search, LSP and process output; hook/watch events; MCP
metadata, schemas and results; web material; child outcomes; history and summaries.
The envelope survives turns, serialization, resume, delegation and compaction.
If precise dependencies are unavailable, a summary or child output conservatively
inherits restrictions of all inputs it could access. Legacy content without
metadata is unknown/restricted, not implicitly public or trusted. Missing metadata
must not trigger a trust upgrade, including after mode toggles or cache reuse.

### D4. Action and process boundary

Preserve the tool lifecycle: PreHooks, then Plan/Security/Permission/Ask checks,
then Run, then PostHooks. Security sees the final rewritten arguments. Changes to
arguments, data version, recipient or relevant policy invalidate the approval.
PostHook stop semantics are preserved; post-execution detection cannot undo an
already completed disclosure. A PreHook process is itself an action to control
before it starts, not an exemption because it precedes the main tool's gate.

Grants default to one use and bind data identity/version, sink, action and expiry
or use count. A task grant is a separately confirmed limited extension, not an
implicit session-wide allow. Expired, revoked or concurrently consumed grants
cannot be reused; cached detector results never cache authority. User decisions
must explain source, destination, operation, scope and uncertainty without
printing secrets. Hard denies and ordinary permission checks remain distinct.

Cover builtins, MCP calls, hook launches, watches and child actions at their real
boundaries. Validate nested action arguments when they describe recipients or
data. MCP discovery can involve a process or network request; it is not assumed
inert. A watch outlives the tool call that creates it and needs lifetime-aware
handling. Read-only child roles do not imply confidentiality.

V1 mediates visible harness actions and known dispatches. It cannot infer all
filesystem/network effects of arbitrary shell code or server behavior. Stage two
must design process-tree ownership, restricted environment, filesystem/network
policy and explicit unsupported-platform behavior before claiming process DLP.

### D5. Every model dispatch is an egress boundary

Apply recipient policy at the actual outbound boundary for main turns, children,
web readers, guards, summarizers/compaction, retries, provider fallbacks and profile
switches. Inspect the effective payload, including system material, history,
tools, attachments and provider-side conversation state. Inspecting only the last
message or only a top-level engine call is insufficient.

A changed endpoint, redirect or fallback recipient requires a fresh applicable
recipient decision before transmission. If a transport cannot mediate a redirect
safely, refuse it rather than following it implicitly. Hidden remote context must
not be treated as clean because the local request is small; reset/rebuild or stop
when its provenance cannot be established. Every actual attempt revalidates grants
against the effective payload and sink, including after queueing or retries.

### D6. Guards, quarantine and escalation

Inspect both material and proposed actions. A guard receives only the minimal
permitted fragment and context necessary for that check, not the parent history.
It has no executable tools. Its structured result uses bounded reason codes and
positions, not raw secret quotations. Invalid JSON, unexpected tool calls,
truncation, missing coverage and timeouts produce unknown, never safe. Any numeric
confidence is not a probability unless independently calibrated.

Cheap detection may run automatically on permitted data. A stronger model always
requires separate user confirmation; no automatic escalation is authorized.
External escalation receives only permitted data without original secrets. A
redacted copy's verdict does not prove anything about omitted original content.
Without a trusted profile, offer limited work on allowed inputs or pause; do not
silently send raw secrets to the default model.

Only an explicitly trusted local/own isolated guard may inspect raw secrets before
encrypted storage exists. Do not persist those inputs in ordinary history, logs,
audit, debugging, temporary files or job transcripts. Until protected persistence
exists, quarantine must be bounded nonpersistent storage or a restricted reference
to existing material; inability to retain it safely pauses/requires reacquisition.
Do not claim protection from OS memory inspection or swap. Observe may retain safe
potential-quarantine metadata but must not block solely on model suspicion.

In enforce, quarantine comes before ordinary consumption and remains until a
permitted user decision. Do not feed an automatically cleaned summary to the main
agent as a substitute for approval. Unavailable mandatory checking pauses the
relevant action. Cancellation releases queue slots and transient material; stale
results cannot release quarantine or grant authority.

Bound input, output, queue size, concurrency, time, retries and cache retention.
Do not recursively guard guard results. Cache keys bind content version, checking
context/scope, model identity and policy version; incomplete checks cannot cover
omitted material. Revalidate grants on use. Audit is local metadata only, with no
raw content/secrets, minimized paths/addresses, and explicit export approval.

### D7. Trusted profiles and encrypted storage

Only local or self-operated profiles are eligible for explicit trust. Before
acceptance, warn that the profile can see secrets, its server can log or forward
them, and injection or model errors remain possible. The harness cannot certify
server isolation. Bind acceptance to provider, endpoint and model; show a visible
trust label; changes require new acceptance. Trust is not inherited by other
profiles, even if they use the same role or provider family.

The final contract applies that profile's trust in every role. Rollout restricts
raw-secret access to the isolated guard until encrypted storage is implemented;
only then can main agents, children and summarizers use raw sensitive context.
No role may silently use a default external profile as a substitute.

Encrypt the entire sensitive session and its derivatives, not only matched secret
strings. Cover history, summaries, child transcripts, temporary artifacts, crash
recovery and backups; inspect exports/debug paths and any other persistence sink.
Use standard authenticated encryption. An OS secure store or a separate passphrase
protects key access; no adjacent plaintext key or plaintext fallback. Exact format,
libraries, KDF/parameters, nonce/key lifecycle, migration and recovery behavior
require the storage design task and its approval, not guesses in implementation.
Failure to access keys stops sensitive persistence; loss of the key may make the
session unrecoverable. Do not promise secure deletion of prior plaintext copies.

An already encrypted session stays encrypted when security is off and may continue
without guards. This is a storage-format invariant, not active transmission
protection. Make that distinction visible. New ordinary off-mode sessions retain
legacy storage behavior. Turning off must not export/decrypt persisted history.
Encryption does not protect against a compromised running host or server logs.

### D8. Moving sensitive context to an untrusted profile

With security enabled, a sensitive-to-untrusted transition requires an explicit
choice: start a clean context from permitted data; prepare a redacted transfer
with a mandatory warning; or cancel. The warning must say that removing known
secrets does not guarantee removal of all sensitive or derived information.
Known remaining secrets must not be sent. Neither a summary nor redaction earns
automatic declassification. Resolve the actual recipient and all retained local
or remote context before dispatch. Trust revocation and profile fallback use this
same boundary, not a special bypass.

While off, guards and security-specific transfer dialogs are disabled. Continuing
an encrypted session without transmission protection must be visibly disclosed;
encryption alone does not make an untrusted recipient safe.

### D9. Evidence, targets and rollout

Target added guard-decision latency is p95 at most one second for ordinary checks
and five seconds for explicitly approved strong checks, including queue/network
and excluding human waiting time. These are goals, not measured guarantees. Track
p50 and p95 separately and disclose the definition and coverage of a check.

Target false stops are at most 5% of benign tasks, not 5% of fragments. Report
benign task completion, questions/escalations, cost and unexamined material as well
as attacks. Observe measures potential stops, not prevented attacks. There is no
agreed monetary budget or chosen model. Technical limits still apply, and this
planning approval authorizes no paid or live-model tests.

An acceptable attack-success threshold, sample sizes, repeated-attempt protocol
and model/version selection remain explicit readiness decisions. A low detector
false-positive rate alone cannot authorize enforcement. Historical small guard
experiments are motivation, not evidence of this system's quality or a model
recommendation; this PR runs no model experiments.

Rollout is foundation default-off; bounded observe with isolated guard; protected
storage and all trusted roles; separately opt-in enforcement after accepted eval
evidence; then process isolation as the second major stage. Users can turn off at
any point, subject to the encrypted-storage invariant. The existing permission and
web controls are not removed or weakened by any slice.

## Testing Decisions

Tests assert public behavior, not internal locks, map layout or prompt wording.
Prefer the highest existing engine/executor/provider/session seams; use the new
policy interface for deterministic decisions and real adapters for integration.
Existing permission decisions, argument rewriting, cancellation, web quarantine,
child delivery and session round-trip tests are prior art, not proof of the new
contract. Every slice includes its own UI/headless explanation and regressions.

| Scenario | Required observation | Contract |
|---|---|---|
| S01: absent setting, off and explicit bypass | Zero new guard/scan/audit/shadow calls; legacy controls remain; bypass alone does not disable security | D1 |
| S02: first enablement and forged consent | Observe selected; hard deny still denies; text cannot change settings/trust/grants | D1–D3 |
| S03: malicious ingress and missing metadata | Every listed ingress has host provenance; forged wrappers and legacy metadata cannot upgrade trust | D2–D3 |
| S04: compact, restart and delegate | Restrictions survive derived outputs, history persistence and child outcomes | D3 |
| S05: changed arguments or recipient | Approval cannot replay across versions/sinks; race/revocation/use limits respected | D4 |
| S06: issue-to-private-file-to-public-PR | Controlled publication sink receives no forbidden payload; hard deny is not a model decision | D2, D4 |
| S07: all model roles and attempts | Capture full fake-provider payloads; main/child/Compact/reader/guard/retry/fallback cannot bypass policy | D5 |
| S08: guard failure or partial coverage | Invalid output, tool calls, truncation and timeout are unknown; enforce pauses; observe records nonblocking signal | D6 |
| S09: strong escalation | No call before distinct confirmation; refusal/headless stops where necessary; original secrets absent externally | D6 |
| S10: guard-only trusted stage | Raw input reaches only accepted isolated guard; inspect every output/storage capture for synthetic canaries | D6–D7 |
| S11: trusted roles and profile change | Warning/identity binding visible; other profiles do not inherit trust; role rollout respects storage readiness | D7 |
| S12: encrypted artifacts and key failures | Entire synthetic sensitive session/derivatives protected; tampering and unavailable keys fail without plaintext fallback | D7 |
| S13: turn off during pending work | Cancel checks/asks; no automatic replay; encrypted continuation remains encrypted with no guards | D1, D7 |
| S14: untrusted transition | Clean/redacted/cancel choices; required warning; known secrets absent; hidden retained context cannot bypass | D5, D8 |
| S15: safe observability and resource pressure | No secret in logs/audit/errors/temp; bounded queues/cache; cancellation releases resources | D6–D7 |
| S16: evaluate attacks and benign tasks | Synthetic secrets and controlled sinks measure actual leak/action outcomes, task false stops and repeated attempts | D9 |
| S17: process-stage boundary | Demonstrate and disclose V1 limits; later isolation design defines per-platform acceptance before implementation | D4, D9 |

The adversarial corpus spans all ingress families, multilingual/long/encoded
content, quiet answer poisoning, approval spoofing, cross-turn persistence,
compaction, delegation, supply-chain hooks/MCP and retry loops. Controlled sinks
measure actual disclosures and actions rather than guard labels. Keep deterministic
regressions independent of live models; version attacks and prevent silent weakening.
Model trials require a separate authorized run and a report of corpus, versions,
repetitions, uncertainty, coverage, latency and cost. No paid tests by implication.

For implementation, run checks only on changed packages/files and at most one
scoped lint invocation before commit. Full repository gates belong to CI. For
this documentation PR, validate document links, registry schema, dependency DAG,
status/frontier consistency and coverage of approved decisions; run no Go gates.

## Out of Scope

- Implementing the feature, choosing a guard model or running paid/live evals in this PR.
- Automatic strong-model escalation or treating a detector verdict as approval.
- Proving arbitrary shell/MCP-server network behavior safe in V1.
- Certifying local/own servers, OS isolation or protection from a compromised host.
- Inventing cryptographic primitives, guaranteeing secure deletion or key recovery.
- Automatically declassifying summaries, redacted output or trusted-model answers.
- Merging this PR or closing implementation tasks on the strength of a design.

## Further Notes

The architecture investigation used repository baseline e08fd0e79f6e5f7ac6ba38176cd0b3fc0801d372.
The documentation branch starts from 5181f3dd3cbb085e390d375eb8baeaf31d3acb11;
the intervening change adds an Ollama reasoning-field alias, not this security
layer. Source observations identify integration gaps, not tested runtime exploits.

Existing behavior to preserve includes permission gating after argument rewriting,
web framing/turn taint and reader quarantine. These do not provide generic durable
provenance or all-recipient egress mediation today. Existing host-only delivery
metadata is not a security label; readonly roles and command inspection are not
sandboxes. Recheck integration details when implementing rather than treating this
baseline as a permanent description of the code.

The agreed tradeoff favors a small policy interface and conservative provenance
over many caller-specific filters. This improves reviewability and fail-closed
behavior but can increase questions and retained restrictions; task-level evals
must expose that cost. Encryption and explicit trust increase complexity but gate
raw-secret access rather than replacing it with a comforting detector score.

The [ticket map](harness-security-tickets.md) reuses the existing dataflow and eval
tasks, preserves their parent epic, and separates storage/process research from
implementation. Planning publication is not implementation delivery. All feature
tasks remain open, and the planning task remains in progress until PR review/merge.

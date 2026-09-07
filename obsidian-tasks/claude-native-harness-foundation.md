---
id: claude-native-harness-foundation
title: Explore Claude-native sessions and harness practices as a cozyphi foundation
status: todo
priority: medium
model_level: very_high
task_type: epic
tags:
    - goal
    - claude
    - harness
    - sessions
    - idea
acceptance_criteria:
    - Before implementation, document an evidence-backed feasibility decision for full bidirectional session/context continuity; explicitly enumerate recoverable state, separately configured state, and unavailable state. A summary handoff does not satisfy the requirement.
    - Demonstrate a controlled Claude → cozyphi → Claude → cozyphi round trip on disposable sessions, including assistant/user history and tool calls/results; verify actual continuation behavior, not merely successful file parsing.
    - Evaluate Claude-native session storage as a potential primary format for cozyphi and Claude-compatible tool contracts as potential native interfaces, without assuming either decision in advance.
    - Evaluate adoption of Claude harness practices against observable reliability, context retention, recovery and usability outcomes; distinguish benefits of harness behavior from model capability and serialization compatibility.
    - Prove or explicitly identify limitations for compaction, branching/resume, unknown records, interrupted tool turns, external configuration, version changes and writer ownership; retain recoverable original sessions.
    - Preserve model choice and user authority. Any proposed changes to current security/edit guarantees require explicit review and approval before implementation; no implicit permission bypass.
    - After feasibility evidence is reviewed, produce a separately approved design and implementation breakdown. If the required continuity is infeasible, return the precise limitation and a user decision rather than silently substituting summaries or a terminal tab.
verification_plan:
    - 'Review this idea brief against the discussion: motivation exceeds compatibility; full bidirectional continuity is required; architecture and implementation are not selected.'
    - When research is authorized, recheck the recorded official source revisions and supported contracts, and classify each finding as documented behavior, source evidence, experimental evidence or unknown.
    - When experiments are authorized, validate an isolated native-record preservation baseline and then a Claude → cozyphi → Claude round trip with synthetic history and tool calls/results; do not use private production sessions.
    - Evaluate context/configuration completeness, compaction, branching, versioning and safety separately; report exact limits rather than calling transcript replay full-context equivalence.
    - Obtain a user decision on feasibility and scope before drafting the architecture, choosing migration strategy or creating implementation tickets.
created_at: "2026-09-07T14:16:07.609437Z"
updated_at: "2026-09-07T14:16:07.609437Z"
---

## Body

**Status and purpose**

Idea-stage epic recorded from the discussion on 2026-09-07. The current request authorizes recording the idea only: do not start architecture design, implementation, migration, experiments or child-ticket decomposition. The acceptance criteria below describe future epic outcomes, not work already completed. No storage format, SDK binding, process topology or tool API is selected by this note.

**Problem and motivation**

The user's premise is that Claude Code is currently the strongest harness and should be treated as a reference for improving cozyphi, not merely as another model endpoint or an external consultant. This is a product direction, not a benchmark result established by the investigation.

The goal is broader than compatibility: examine whether cozyphi should adopt Claude's session representation and relevant harness/tool conventions as its own foundation where justified, while retaining cozyphi's interface and freedom to use other models. Continuing the same work in native Claude would be a valuable consequence of that common foundation, not the sole reason for adopting it.

A terminal tab that merely embeds Claude Code offers placement convenience but does not by itself solve shared context or improve cozyphi's harness. A summary handoff loses information and does not satisfy the user's explicit requirement for the full context in both directions.

**Desired user outcome**

A user can work in Claude, continue in cozyphi using another supported model, and return to Claude without manually reconstructing the conversation, losing relevant tool results, or replacing the working history with a summary. The receiving harness must understand prior work, not merely display an imported transcript. The user should be able to benefit from Claude's harness when using Claude and from proven Claude-inspired practices when using cozyphi's own engine.

This is an intended outcome whose technical feasibility remains open. A shared JSONL format alone does not recreate Claude's runtime behavior or model quality.

**User scenarios**

1. As a user, I want to open work begun in Claude in cozyphi and retain its conversation and tool-result evidence, rather than explain the task again.
2. As a user, I want to switch to another model in cozyphi, perform useful work, and have that new work understood when I return to Claude.
3. As a user, I want the round trip to preserve history as structured work, not insert the previous conversation as one large user message.
4. As a user, I want tool calls, results and failures to remain intelligible across harnesses; matching tool names alone is insufficient.
5. As a user, I want long-running and compacted sessions handled explicitly, without pretending that an existing compact summary contains the discarded original context.
6. As a user, I want branching and continuation to preserve the chosen line of work rather than accidentally join unrelated branches or child conversations.
7. As a user, I want restart and handoff to avoid duplicated tool execution, silent writes by two processes, or corruption of an original Claude session.
8. As a user, I want unsupported session versions or records to produce an actionable limitation rather than silent data loss.
9. As a user, I want control over models, tools and permissions to remain explicit when changing harnesses.
10. As a user, I want improvements to cozyphi's harness to be justified by observed behavior, not merely by copying a file format.
11. As a user, I want any state that cannot be transferred to be disclosed before a switch, not concealed behind a claim of full continuity.

**Meaning of full context: requirement to investigate, not a claimed capability**

Separate persisted transcript, the active post-compaction conversation, the actual next model request, and the runtime/configuration used to construct that request. Determine which of the following are stored, independently configured, regenerated, or opaque: user/assistant messages; tool-use/tool-result blocks and their links; system instructions; project instructions and memory inputs; tool definitions and effective permissions; compaction records and active history boundaries; session/parent identifiers; branch and child-session relationships; attachments and references to external artifacts; unresolved operations and other continuation metadata.

Do not infer that every category is present in Claude JSONL. Reading all lines is not proof of obtaining the exact next request. Conversely, the absence of a public export method is not proof that compatible continuation cannot be implemented. Cross-model portability of any provider-specific opaque blocks must also be established rather than assumed.

The user asked for full transfer, not a fallback summary. If exact equivalence cannot be supported, document the precise boundary and seek a product decision before reducing scope.

**Directions worth evaluating later — not architecture decisions**

- Claude-native session records as the primary persistent representation of cozyphi, rather than a peripheral import/export format.
- A shared or compatible persistence mechanism through the SDK's SessionStore, or another evidenced route.
- Adapting cozyphi's tool interfaces and result conventions toward Claude where that improves both continuity and harness quality, rather than preserving current interfaces by default.
- Keeping cozyphi-specific metadata without corrupting or confusing native Claude records; no sidecar or database design is selected here.
- Reusing accessible, proven Claude harness practices while distinguishing public SDK mechanisms from behavior inside the separate Claude Code engine.
- Retaining native cozyphi execution with other models; SDK integration is not equivalent to replacing the ordinary LLM provider interface.

**What the source investigation found**

On 2026-09-07 an agent cloned the official repositories into ~/src/claude-agent-sdk-python and ~/src/claude-agent-sdk-typescript. Reported source revisions:

- Python: https://github.com/anthropics/claude-agent-sdk-python at efd4d865ef1795daffee3cd24cce45307aed8a51.
- TypeScript: https://github.com/anthropics/claude-agent-sdk-typescript at 69f318f92fd5bd21614b35d5c35ef9e35445aecb.

The investigation was static source analysis, not a live round-trip experiment. Official web documentation retrieval was unsuccessful (blocked requests/403); do not present those pages as verified sources. Recheck version-sensitive findings before future design.

The Python SDK exposes SessionStore.append/load, ClaudeAgentOptions.session_store, import_session_to_store and resume. Import replays a local native Claude transcript into a store. Resume from the store materializes records into temporary JSONL for the Claude Code resume mechanism. This is a concrete integration point, not merely a rendered-history API.

Crucial source contract: the entry union is internal, adapters should treat entries as pass-through blobs, and returned entries must be deep-equal to appended entries. Consequently, preservation and replay of native records are supported; construction of new Claude-compatible records from cozyphi work is not proven by that contract.

Source evidence: Python src/claude_agent_sdk/types.py:1515–1528 and :1631–1643; src/claude_agent_sdk/_internal/session_import.py:29–68; src/claude_agent_sdk/_internal/session_resume.py:130–200. These are pointers at the recorded revision, not permanent line numbers.

get_session_messages provides a projection of history rather than every internal record. get_context_usage reports context usage, not the full textual request. System-prompt configuration is separate from the session store. See Python src/claude_agent_sdk/_internal/sessions.py:1023–1124 and src/claude_agent_sdk/types.py:764–811, :1967–1975.

The Python repository contains SDK implementation and session helpers, but execution delegates to a separate Claude Code process. The checked TypeScript repository contains documentation/changelog and SessionStore adapter examples, not the complete implementation of the published SDK or the underlying Claude engine. Do not infer hidden context-assembly behavior from the wrapper.

**Unknowns and risks to resolve before design**

- Whether Claude accepts foreign-generated history with tool calls/results on resume, and whether it interprets it as intended.
- Which internal record fields, parent chains and metadata are essential for valid continuation, including compaction and branching.
- Whether imported history preserves only conversation continuity or the exact active context the user expects.
- How separately supplied prompts, tools, settings, project files and permissions affect equivalence.
- Whether adapting tool contracts can retain current safety guarantees, especially session-scoped hashline capabilities; serialized tool history must not automatically become a fresh edit grant.
- Compatibility across Claude versions and an upgrade policy for an internal record format.
- Recovery from interruptions and competing writers, without manipulating live private sessions or replaying side effects.
- Applicable SDK/engine licensing and authentication terms; standalone CLI subscription behavior must not be assumed to authorize every integration mode.
- How to assess harness improvements independently from Claude model capability.

**Relationship to existing work**

Related task: [[design-agent-backends]], a completed design-only task; related proposed document: doc/agent-backends.md. The source investigation found that its proposed tool-execution contract keeps tools in cozyphi and uses summary handoff for cross-backend transfers. That does not meet this epic's full-continuity goal and should not silently determine its architecture.

Related area: [[claude-consult-tool]] and its existing implementation tickets. Calling Claude as a consultant is a different scope. This epic neither closes nor reparents those tasks and does not declare their designs obsolete. Reconcile overlaps when design is explicitly requested.

**Current non-goals**

No implementation plan, package/interface design, storage migration, tool renaming, UI choice, PTY implementation, SDK dependency addition, or mandatory replacement of the current engine. No claim that matching Claude storage yields its reasoning quality. No paid/live Claude experiment under this recording request. No weakening of permission gates, hashline guarantees or isolation without a later explicit decision and approval. Do not reopen the previous design task or mark this epic ready for implementation merely because the idea is recorded.

**Future validation intent**

Evaluate actual externally observable continuation using disposable sessions and synthetic non-sensitive fixtures. Preserve originals. Cover both directions, ordinary and tool-bearing turns, native compaction, branch selection, restart, unsupported records and version differences. Distinguish an offline parser/store test from a live Claude continuation test; success of one does not establish the other. Any live experiment, dependency setup, design and implementation scope should be agreed when this epic is taken up.

## Acceptance Criteria

- Before implementation, document an evidence-backed feasibility decision for full bidirectional session/context continuity; explicitly enumerate recoverable state, separately configured state, and unavailable state. A summary handoff does not satisfy the requirement.
- Demonstrate a controlled Claude → cozyphi → Claude → cozyphi round trip on disposable sessions, including assistant/user history and tool calls/results; verify actual continuation behavior, not merely successful file parsing.
- Evaluate Claude-native session storage as a potential primary format for cozyphi and Claude-compatible tool contracts as potential native interfaces, without assuming either decision in advance.
- Evaluate adoption of Claude harness practices against observable reliability, context retention, recovery and usability outcomes; distinguish benefits of harness behavior from model capability and serialization compatibility.
- Prove or explicitly identify limitations for compaction, branching/resume, unknown records, interrupted tool turns, external configuration, version changes and writer ownership; retain recoverable original sessions.
- Preserve model choice and user authority. Any proposed changes to current security/edit guarantees require explicit review and approval before implementation; no implicit permission bypass.
- After feasibility evidence is reviewed, produce a separately approved design and implementation breakdown. If the required continuity is infeasible, return the precise limitation and a user decision rather than silently substituting summaries or a terminal tab.

## Verification Plan

1. Review this idea brief against the discussion: motivation exceeds compatibility; full bidirectional continuity is required; architecture and implementation are not selected.
2. When research is authorized, recheck the recorded official source revisions and supported contracts, and classify each finding as documented behavior, source evidence, experimental evidence or unknown.
3. When experiments are authorized, validate an isolated native-record preservation baseline and then a Claude → cozyphi → Claude round trip with synthetic history and tool calls/results; do not use private production sessions.
4. Evaluate context/configuration completeness, compaction, branching, versioning and safety separately; report exact limits rather than calling transcript replay full-context equivalence.
5. Obtain a user decision on feasibility and scope before drafting the architecture, choosing migration strategy or creating implementation tickets.

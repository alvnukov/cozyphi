---
id: refactor-prompt-snapshot
title: Render prompts from explicit session-context snapshots
status: todo
priority: medium
model_level: high
task_type: refactor
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - Engine construction does not perform hidden prompt-source IO or panic because cwd/context discovery failed; source acquisition errors are handled before pure rendering.
    - The renderer consumes an explicit immutable snapshot of effective session cwd/workspace, instruction files, skills and server metadata rather than consulting ambient process cwd.
    - Root and child prompts retain their effective project through construction, reconfiguration, resume and rebuild; the narrow cross-project correction owned by multisession-projects is reused rather than implemented twice.
    - Pure prompt rendering is testable without filesystem access; instruction path formatting is safely escaped and facts measure the supplied render rather than triggering a second load.
verification_plan:
    - Inspect any landed multisession-projects context correction; assign non-overlapping changes before editing shared prompt code.
    - Test pure render and facts from supplied snapshots with no filesystem access, including escaped paths and empty/missing inputs.
    - Test source acquisition failures return actionable errors without engine-construction panic, and explicit A/B root/child contexts survive relevant rebuild paths using controlled adapters.
    - Run format/build/tests only for changed prompt/agent packages; at most one scoped lint. No live provider calls or repository-wide checks.
created_at: "2026-08-23T15:17:22.11967Z"
updated_at: "2026-09-06T14:02:53.931508Z"
---

## Body

**Problem and original intent.** Prompt building mixes rendering with source discovery and can panic on cwd/template failures; project context formatting also needs safe path escaping. The original 2026-08-23 task identified child prompts being stamped with process cwd rather than their WorkDir and proposed an immutable snapshot of cwd/root/files/skills/servers. Preserve that refactor intent without relying on its old line numbers or legacy ~/.phi path spelling.

**SOURCE refresh (2026-09-06).** At 1a4cf31, relevant source unchanged at 05ce564, Engine.systemPrompt calls prompt.BuildWithFacts without session cwd. BuildWithFacts calls currentDir for the declared cwd and loaded project instructions; currentDir calls os.Getwd. This confirms the cross-project context problem by source, not a live execution test. Pure-render separation, error paths and escaping still need their own verification.

**Ownership agreement.** [multisession-projects](multisession-projects.md) owns the narrow explicit-session-context correction as the first internal step of its cross-project vertical slice. That slice must not wait for this entire refactor. This task owns the remaining pure snapshot/source-acquisition separation, hidden IO/panic removal and formatting safety; inspect the landed project slice before implementation and reuse its context plumbing. Do not create a second prompt policy, parallel renderer or duplicate fix. A reciprocal related-work link is not a circular blocking dependency.

**Design constraint.** Capture coherent effective context outside rendering, report acquisition errors actionably, and render/facts from that snapshot without reading the filesystem again. Keep session/project ownership explicit across root and child lifecycles. Avoid an unrelated broad engine rewrite solely to make rendering hermetic.

**Blocked by:** None as a full task dependency; coordinate the shared prompt code with multisession-projects before taking implementation ownership. This backlog update does not start or complete the refactor, and its parent remains the enterprise code review epic.

## Acceptance Criteria

- Engine construction does not perform hidden prompt-source IO or panic because cwd/context discovery failed; source acquisition errors are handled before pure rendering.
- The renderer consumes an explicit immutable snapshot of effective session cwd/workspace, instruction files, skills and server metadata rather than consulting ambient process cwd.
- Root and child prompts retain their effective project through construction, reconfiguration, resume and rebuild; the narrow cross-project correction owned by multisession-projects is reused rather than implemented twice.
- Pure prompt rendering is testable without filesystem access; instruction path formatting is safely escaped and facts measure the supplied render rather than triggering a second load.

## Verification Plan

1. Inspect any landed multisession-projects context correction; assign non-overlapping changes before editing shared prompt code.
2. Test pure render and facts from supplied snapshots with no filesystem access, including escaped paths and empty/missing inputs.
3. Test source acquisition failures return actionable errors without engine-construction panic, and explicit A/B root/child contexts survive relevant rebuild paths using controlled adapters.
4. Run format/build/tests only for changed prompt/agent packages; at most one scoped lint. No live provider calls or repository-wide checks.

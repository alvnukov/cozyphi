---
id: prioritize-visible-successor-anchors
title: Show changed-range anchors before surrounding successor context
status: todo
priority: medium
model_level: medium
task_type: bug
parent_id: reliable-model-file-edits
tags:
    - review
    - tools
    - reliability
acceptance_criteria:
    - The display budget prioritizes edited range endpoints/regions before surrounding unchanged context.
    - A 20-line replacement at 100..119 exposes the changed tail within the same bounded budget.
    - Multiple distant edits receive a deliberate fair display allocation or an explicit targeted read instruction.
    - Every displayed anchor is actually authorized; long-file and cap behavior remains explicit.
verification_plan:
    - Test public EditTool/WriteTool output for mid-file replacements, distant edit ranges and files longer than display/grant caps.
    - Verify printed anchors authorize the intended next edit and output stays bounded.
created_at: "2026-09-05T06:42:03.971556Z"
updated_at: "2026-09-05T06:42:03.971556Z"
---

## Body

Confirmed on main 0032d62 by TestEpicAuditSuccessorDisplaysChangedTail, helper command d767aab909165d38b143cbc5ab6644c7. successorGrantFor expands spans by ±25 lines, then writeSuccessorBlock prints only the first 40 anchors in ascending order. Replacing lines 100..119 returns anchors 75..114: the final five modified lines are hidden while 25 untouched preceding lines are shown. Multiple separated edits favor the first window; post-write output shows only the first 40 lines. The stored 512-anchor grant does not help a model that cannot see the anchor. This is a usefulness gap, not permission bypass. Keep output bounded but prioritize changed endpoints/regions and report precise refresh windows for omitted regions.

## Acceptance Criteria

- The display budget prioritizes edited range endpoints/regions before surrounding unchanged context.
- A 20-line replacement at 100..119 exposes the changed tail within the same bounded budget.
- Multiple distant edits receive a deliberate fair display allocation or an explicit targeted read instruction.
- Every displayed anchor is actually authorized; long-file and cap behavior remains explicit.

## Verification Plan

1. Test public EditTool/WriteTool output for mid-file replacements, distant edit ranges and files longer than display/grant caps.
2. Verify printed anchors authorize the intended next edit and output stays bounded.

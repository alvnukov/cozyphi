---
id: drop-default-effort-row
title: Drop the default row from the reasoning-effort menus
status: done
priority: medium
model_level: medium
task_type: bug
tags:
    - tui
    - reasoning
acceptance_criteria:
    - No effort picker offers a choice that is not one of the model's own levels.
    - The composer never labels an unset effort with a level name.
    - Clearing a selection stays possible through an empty effort (model switch, resumed session).
verification_plan:
    - Unit tests over the shared flow, the palette page, the settings and plan pickers.
    - Composer control test for a model with levels and no selection.
    - Controller test that the literal token is rejected while an empty effort clears.
created_at: "2026-09-07T16:30:00.000000Z"
updated_at: "2026-09-07T16:52:00.000000Z"
---

## Body

The pickers prepended `default` to every effort page (internal/tui/modelflow),
and the composer showed the same word as the current effort whenever none was
selected. It was the clear token, not a depth: the page listed one entry more
than the model owns, and the control read as though a level named `default`
were on its way to the provider. It never was — every path mapped it to the
empty effort — but the word confused readers of the menu and of the main
screen alike, and the same observation is recorded in
model-specific-openai-codex-efforts.

Fixed in 8581f3e: `Flow.Efforts` returns the model's own levels only and
`SelectEffort` rejects anything else; the palette page, the settings pane, the
sidebar step picker and the plan editor follow it; `SetModelEffort` keeps the
empty effort as the clear path and now rejects the literal token; the composer
names the control `effort` while no level is selected. doc/tui.md and the
changelog record the new behavior.

## Acceptance Criteria

- No effort picker offers a choice that is not one of the model's own levels.
- The composer never labels an unset effort with a level name.
- Clearing a selection stays possible through an empty effort (model switch, resumed session).

## Verification Plan

1. Unit tests over the shared flow, the palette page, the settings and plan pickers.
2. Composer control test for a model with levels and no selection.
3. Controller test that the literal token is rejected while an empty effort clears.

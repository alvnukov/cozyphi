# TUI interaction standard

Every list-shaped surface of the TUI speaks one dialect. A user who has
learned the context browser already knows the plan editor, the settings
pane, the help screen and every picker: the same keys move, the same keys
act, the same footer explains them. This file is the contract; the shared
state machines behind it live in `internal/tui/browse`, and every hint a
pane renders comes from the `internal/tui/keys` catalog.

The rule for new code: a pane owns its rows and its actions, nothing else.
Motions, cursor-following, scrolling, one-shot notices and y/n
confirmations come from the kit. If a pane needs a behavior the kit lacks,
the kit grows — a private reimplementation is a bug by definition, because
it will drift.

## The motion dialect

| Keys              | Motion                                        |
| ----------------- | --------------------------------------------- |
| `↑` `↓`, `j` `k`  | one row (counts apply to `j`/`k`: `3j`)       |
| `gg`, `Home`      | first row                                     |
| `G`, `End`        | last row; with a count, that row (`12G`)      |
| `Ctrl+U` `Ctrl+D` | half a screen                                 |
| `PgUp` `PgDn`     | a screen, keeping one row of overlap          |
| wheel             | three rows per notch, moving the window       |

Counts accumulate digit by digit and are cleared by any key that is not
part of the dialect. A half-typed `gg` is cleared the same way. Modified
arrows (`Shift+↑↓` and friends) are never motions — they stay with the
pane, which may bind them to pane-specific work, under one rule:
`Shift+↑↓` always means extending a selection, because that is what it
means in every list outside this TUI too. Reordering — moving the selected
item up or down its list — is `Alt+↑↓`, the editor line-move convention. A
pane with reordering but no multi-selection answers `Shift+↑↓` with a
notice naming `Alt+↑↓` instead of quietly mutating.

On a cursor pane the wheel moves the window and leaves the cursor where it
is; keyboard motions move the cursor and the window follows it. The cursor
never rests on an unselectable row (a header, a blank spacer): stepping
skips them, jumps snap to the nearest selectable row in the direction of
travel.

## Selection and activation

`Enter` opens or activates the selected row; `Space` is a synonym in every
list. What "activate" means is the pane's business — open a detail, toggle
a flag, pick a value — but the key is always the same pair.

A choice list (picking a type, a model, an event) opens with the cursor on
the current value, not on the first row. `Enter` chooses and goes back,
`Esc` goes back without choosing, and delete keys answer with a notice
instead of doing nothing.

## Choice modals

A modal with a handful of fixed options — a permission ask, a continue
prompt, a question — is a choice list with two deliberate differences.
Digits pick an option directly, because the rows are numbered on screen;
that is why these modals have no counts. And the selection wraps at the
edges (`browse.Ring`), because a list with no scroll turns a dead edge
into a key that appears broken. Everything else holds: `Enter` takes the
highlighted option and `Space` is its synonym, `j`/`k` step, `Esc` backs
out one level, and a key the modal cannot use answers with a hint naming
the keys that work. A modal whose list is filtered by typing (the
provider picker) navigates with arrows only — letters belong to the
filter.

## Deletion and confirmation

`Del` deletes the selected item wherever deletion exists. A pane whose
whole job is managing entries may add `d` as a synonym; `Backspace` never
deletes — it answers with a notice naming the key that does.

Destructive actions arm an inline y/n question in the footer, styled as a
warning, that names its exact target: `Delete step 2, "wire the pane"?
(y/n)`, with long values previewed and truncated. `y` fires, `n` and `Esc`
cancel, and any other key cancels without firing — acting elsewhere
withdraws the question. Only one question can be armed at a time: arming a
new one replaces the old, so a double `y` can never fire two different
actions. These transient prompts render their own `(y/n)` tail and stay
out of the keys catalog (see the catalog's package doc).

## Notices

When a key is recognized but refused — `Backspace` in a list, `Del` in a
choice list, `Shift+↑↓` on a non-step row — the pane says so in the
footer, in warning style, naming the key that works. A notice lives for
exactly one keypress: the next key, whatever it is, clears it.

## Footers and help

The footer hint row and the `/help` screen render from the
`internal/tui/keys` catalog and from nothing else; a literal hint string
in a pane is a bug. Bindings with an empty `Hint` are documentation-only:
they appear on the help screen but not in the footer, which keeps the
footer to the handful of keys worth a permanent reminder. `keys_test.go`
pins the exact rendered rows.

## Editing and applying

Edits accumulate in a draft; `Ctrl+S` applies the draft, `Esc` backs out
one level at a time, and an `Esc` that would drop unapplied edits arms a
discard confirmation instead of silently losing work.

Where a draft is kept, it is honest about what it holds. Every row whose
value differs from the durable state wears a `●` dirty marker, and the
header counts them — the total in the header is the number of dots on the
rows. `Ctrl+Z` takes back one logical edit (a saved field, a toggled flag,
a reorder — not one keystroke), `Ctrl+Y` brings it back, and undoing to
the baseline withdraws dirtiness entirely, so `Esc` closes without the
discard question. History that would now lie — after a rebase onto a plan
that moved underneath — is dropped rather than replayed. A choice list
refuses undo with a hint; back out first. Still planned on top of this
(the plan-editor redesign): a bottom inline editor instead of modal text
screens, `/` fuzzy jump and a `.` action menu.

## Wide screens: master and detail

A pane with a detail view splits in two when the panel is at least 86
columns wide: the list keeps the left column and the right column expands
the selected row — the full wrapped value of a field with its length
against the limit, the detail form of a step, the plan overview when the
row has nothing of its own to show. The preview is passive; `Enter` still
opens the detail, which then takes the right column with its own cursor
while the master keeps a passive marker on the row the detail belongs to.
Every list keeps its own cursor, so round trips preserve the selection:
`Esc` from a detail lands on the step it showed, and a choice list —
which takes the full panel even on wide screens, a picker being a modal
question — returns to the row that opened it whether a value was picked
or not.

Focus follows the mouse: clicking a master row while the detail is
focused returns to the list and acts there, clicking a preview row opens
the detail at that row, and the wheel scrolls the pane under the pointer
whether or not it has focus. Below the threshold the pane renders the
single-column layout it always had, with no behavior change.

## Adoption

| Surface        | Status                                                  |
| -------------- | ------------------------------------------------------- |
| helppane       | on the kit                                              |
| ctxpane        | on the kit                                              |
| planedit       | on the kit                                              |
| settings       | on the kit                                              |
| overlays       | on the kit (choice modals — see above)                  |
| sidebar        | on the kit (step motions + picker ring; Ctrl+D means details, so the half-page chords stay out) |
| transcript     | on the dialect (wheel + page keys with the overlap row; plain keys belong to the composer, so there are no letter motions) |

The `Shift+↑↓` clash between the context browser (extend selection) and
the plan editor (move step) is resolved: `Shift` yielded in the plan
editor, where steps now move on `Alt+↑↓` and `Shift+↑↓` answers with a
notice — see the modified-arrows rule above.

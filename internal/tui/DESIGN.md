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

A multi-select picker (the plan editor's step skills) is a choice list
with one deliberate difference: `Enter` toggles the row's `[x]` mark and
stays, so a pick can be taken back at once, and `Esc` means done — the
toggles already live in the draft, so there is nothing to cancel. Each
toggle is one undo step. It is also the one choice list long enough to
earn the `/` jump, because a catalog outgrows a screen where an event
list never does. A value outside the offered set is never blocked and
never silent: an explicit "other" row keeps hand-typed entry possible,
and an unknown name wears a `⚠` mark in the picker and the summary row,
with the save answered by a warning that names it.

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

A modal owns the mouse the way it owns the keyboard. A click on an
option row selects it, a click on the already selected option activates
it — the same two-beat gesture as moving and pressing `Enter` — and a
row's description belongs to its option. The wheel steps the ring
wherever the pointer is. Every other mouse event dies at the modal:
a click that fell through used to resize the sidebar or scroll the
transcript while the user thought they were answering the dialog
(`overlays.HandleAskMouse`, routed in `Editor.Handle` before any
other mouse consumer).

An option that widens a grant explains itself before it fires. The row
under the options always explains the highlighted one — the two-beat
gesture (select, then activate) reads the fine print in between — and
the allow-alls explain in warning style, naming exactly what they
create: the session grant admits it silences every ask until exit, the
persistent grant names the rule key and the file it writes. A choice
that permanently widens permissions is guarded by the standard armed
y/n question (`browse.Confirm`) naming its target file; the write's
outcome comes back as a toast either way, because a rule the user
believes exists but was never written is worse than the error.

A modal's detail is windowed, never cut from state. Collapsed, the
permission ask shows twelve detail rows and a marker naming `v`;
expanded, `v` opens a scrollable window (`↑↓`/`j`/`k`, the wheel at the
dialect's three rows a notch) with markers counting what is above and
below. `Esc` backs out one level — an open detail folds before the ask
denies — and the answer keys keep working with the detail open, because
reading must never block deciding. An edit or write ask shows the diff
it is asking about (`writetool.AskPreview`, attached by the executor
only when a human will look, carried as `permission.Request.Preview`
outside policy evaluation), styled by unified-diff prefix. When the
panel is shorter than the body, detail rows are what give way
(`fitAskBody`): an option pushed off screen is an ask nobody can
answer, so the options and the hint survive at any height.

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

A failed run is a system event, not assistant prose. The transcript
entry opens with the cause and the action that fixes it — bad
credentials point at `/connect`, an overflow at `/compact`, a rate
limit at waiting — keeps the raw provider error as the detail, and
names the retry path: `↑` in the composer recalls the prompt. The
classification lives in `controller/runerror.go`; the `✕ run error`
marker is `block.AssistantBlock`'s `StateError` treatment. A new
error surface reuses the classifier instead of printing `err.Error()`.

## Footers and help

The footer hint row and the `/help` screen render from the
`internal/tui/keys` catalog and from nothing else; a literal hint string
in a pane is a bug. Bindings with an empty `Hint` are documentation-only:
they appear on the help screen but not in the footer, which keeps the
footer to the handful of keys worth a permanent reminder. `keys_test.go`
pins the exact rendered rows.

## Chords and the binding table

The rebindable global chords — help, palette, settings, the plan pair,
the sidebar commands, transcript copy — live in one binding table in
`internal/tui/keys` (`table.go`): command id → chord. `Editor.Handle`
resolves a key event through the table (`keys.GlobalCommand`) and
dispatches on the command id; it never compares a chord itself, and a
pane that owns a table command's action (the palette in the composer)
matches through the same table (`keys.Is`). The catalog's rows for these
commands name the id instead of spelling keys, so the footers, the help
screen and the palette's shortcut column all render the table's current
chords — an override changes the behavior and every place that
advertises it in one move.

The config's `keybinds` section overrides a chord per command id
(`plan-editor: Ctrl+G`; `none` unbinds; a comma separates
interchangeable chords, as in the copy default `Ctrl+Shift+C, Cmd+C`).
It is validated at load: an unknown id, a malformed spelling, or two
commands on one chord fails the start, because a broken override
discovered as a dead key is a debugging session where a config error was
available. Matching is exact on modifiers — Ctrl+P and Ctrl+Shift+P are
different chords — which is what makes the conflict check meaningful. A
chord whose command does not apply right now (Ctrl+A with no plan) falls
through the ladder like any unclaimed key instead of going dead.

Ctrl+P and Alt+P remain a deliberate pair: Ctrl+P opens the plan editor
modal, Alt+P moves focus into the sidebar plan. They are separate
command ids (`plan-editor`, `plan-focus`) with distinct help rows, and
either can be moved or unbound in `keybinds`.

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
refuses undo with a hint; back out first.

Text is edited in place, not in a popup: `Enter` on a field opens an
editor strip along the bottom of the panel — a rule row naming the field
and its length budget (`Edit wire-pane · content · 97/2048`), then the
text itself, growing with its content up to six lines. The list stays
visible above with a passive cursor on the edited row, so the edit never
loses its context. `Enter` or `Ctrl+S` saves the field, `Esc` cancels,
and a value that fails validation keeps the editor open with the reason
on the message line.

## Fuzzy jump and the action menu

A long list earns two more keys. The machinery is kit-owned —
`browse.Jump` holds the query, the ranking and the strip drawing;
`browse.MenuItem` is the menu's row — so every adopter behaves
identically by construction: the plan editor, the settings modal and
the context browser all carry the pair, and a private reimplementation
is a bug by definition. `/` opens a jump
strip at the bottom: every keystroke moves the real selection to the
tightest fuzzy match live — the strip counts the matches, `↑↓` cycle
them, `Enter` keeps what the jump found, `Esc` restores the selection it
started from, and a click keeps the match and acts as usual. `.` opens
an action menu for the selected row: the commands that apply to it, each
naming its chord (`Move step down (Alt+↓)`), plus the always-available
plan commands, with undo and redo appearing only when there is history
to walk. The menu is a choice list like any other — `Enter` runs the
command in the mode the menu came from, exactly as its chord would, and
`Esc` returns without acting. Chords that live in the menu stay out of
the footer; the menu is where they are discovered.

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

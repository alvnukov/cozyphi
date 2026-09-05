# Context loading

CozyPhi has three distinct file-content paths. Keep the choice at the operation boundary; file paths do not select a representation.

## Context resources

Project instructions, memory, and selected plan-step skills enter the model as plain text. They bypass the `read` tool and hashline formatting. At `step_start`, the plan runtime resolves enabled skills in plan order, deduplicates them, and injects each complete `SKILL.md` before the step's first working tool dispatch. If the current call started the step, that call is refused with retry guidance after the context is installed.

## View reads

`read` defaults to `mode:"view"` and opens with an `@read path (N lines, size, showing A-B)` header before the `N|content` lines. The header carries the total line count when the file fit in memory (windowed pages past 8 MiB report size only) and the shown range, so pagination needs no scouting read; a page past EOF drops the range. Line numbers remain available for LSP positions and references; the output contains no `@file` header, file tag, or line hash and grants no edit capability.

## Editable reads

`read` with `mode:"edit"` returns an `@file path#TAG` header followed by `N#HASH|content`. `grep` keeps the same editable hashline output. A session-owned ledger records the exact path, snapshot tag, and returned anchors.

`edit` claims the matching ledger capability before validating or applying an edit. Failure does not consume it: a malformed, stale or otherwise refused attempt releases the claim unchanged, so the fix is to correct the call and retry without reading again. Only an applied edit consumes the snapshot, and it commits a successor grant in its place — the new TAG plus live anchors over the edited regions and their surrounding context, bounded by the constants in `hashline.go` and printed in the result. A successful `write` mints the same kind of grant for the revision it placed on disk, bounded from line 1. Every prior anchor of that path dies with the swap.

The live capability is therefore whatever the latest editable result printed: lines whose `LINE#HASH` anchors appear in the last `read` with `mode:"edit"`, editable `grep`, `edit` or `write` result for that file are editable now; any other line needs a fresh `read` with `mode:"edit"` of that range first. All anchors of one `edit` call must come from one observation of the file — endpoints spliced from two reads refuse with `mixed_grants`.

The hashline validation then checks the whole-file tag and line hashes and applies all ranges atomically. Therefore view output, anchors from another session, replayed capabilities, and stale snapshots all fail closed. An external write changes the file's TAG, and the refusal stands rather than the edit: every refusal renders as `[edit:<code>] <what happened>. Do not retry the same call unchanged. <next step>`, where the next step is the recovery. `doc/edit-capability.md` carries the codes.

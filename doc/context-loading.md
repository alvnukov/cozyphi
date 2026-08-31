# Context loading

CozyPhi has three distinct file-content paths. Keep the choice at the operation boundary; file paths do not select a representation.

## Context resources

Project instructions, memory, and selected plan-step skills enter the model as plain text. They bypass the `read` tool and hashline formatting. At `step_start`, the plan runtime resolves enabled skills in plan order, deduplicates them, and injects each complete `SKILL.md` before the step's first working tool dispatch. If the current call started the step, that call is refused with retry guidance after the context is installed.

## View reads

`read` defaults to `mode:"view"` and returns `N|content`. Line numbers remain available for LSP positions and references; the output contains no `@file` header, file tag, or line hash and grants no edit capability.

## Editable reads

`read` with `mode:"edit"` returns an `@file path#TAG` header followed by `N#HASH|content`. `grep` keeps the same editable hashline output. A session-owned ledger records the exact path, snapshot tag, and returned anchors.

`edit` consumes the matching ledger capability before validating or applying an edit. Consumption is one-shot for every attempt, including malformed, stale, and otherwise failed attempts. The existing hashline validation then checks the whole-file tag and line hashes and applies all ranges atomically. Therefore view output, anchors from another session, replayed capabilities, and stale snapshots all fail closed.

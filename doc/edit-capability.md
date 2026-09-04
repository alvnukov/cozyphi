# Edit capability and recovery

Design contracts for reliable model-facing file edits: typed failure reasons,
safe re-anchoring, successor capability after `edit`, post-`write` capability,
and unique plan-step auto-binding. Source: the `reliable-model-file-edits` epic.
Baseline (227 saved transcripts, `scripts/analyze_edit_errors.py`):
`edit` errors 500/2905 (17.2%), `write` 35/430; classes `stale_anchors` 264,
`no_capability` 117, `plan_gate` 75, `tag_mismatch` 56; blind retries 385;
edit-fail→write escapes 66.

## Principles

1. **Reliability over protocol exam.** The harness mechanically compensates
   unambiguous mechanical mistakes (a uniformly shifted line number, a missing
   bookkeeping field). On any conflict or ambiguity it stops and returns one
   concrete recovery step.
2. **Fail closed, always.** External TAG change, duplicate hashes, mixed
   grants, overlap, unseen anchors, out-of-workspace paths: refuse. No silent
   overwrite, ever.
3. **One deep module owns capability state.** Grants, consumption, re-anchoring
   and successor math live behind one interface; callers never reconstruct the
   reason for a refusal from strings.

## Current seams (facts)

| Piece | Today |
| --- | --- |
| `internal/tools/editledger` | `Ledger.Authorize(path, tag, anchors)`, `Claim(path, tag, anchors) (*Claim, bool)`, `Release(claim)`. Bounds: `maxTrackedSnapshots=16`, `maxGrantsPerSnapshot=4`. A claim removes every snapshot of the path; `Release` restores them unchanged. |
| `internal/tools/readtool` | `read` with `mode:"edit"` calls `ledger.Authorize(path, tag, anchors)` for the shown window. |
| `internal/tools/greptool` | `GrepTool(ledger.Authorize)` — editable grep output authorizes the same way. |
| `internal/tools/writetool/hashline.go` | `EditTool(ledger)` → `runAuthorizedEdit`: claim → `runParsedEdit` (disk TAG check, `ApplyHashlineEdit`, atomic swap behind `unchangedTagGuard`) → `Release` on failure only. |
| `internal/tools/writetool/write.go` | `WriteTool()` takes no ledger; a successful write grants nothing. |
| `internal/plangate` | `Policy.Check(phase, plan, call) Verdict` — miss reasons for invalid/inactive `plan_step`; `exemptBinding` for exempt tools; executor applies verdicts (`SetPlanGate`, `_plan` envelope, start/settle). |

## The capability module

`editledger` deepens into the session's single capability authority. Interface:

```go
Observe(path, tag string, anchors []string)     // read mode:"edit" / editable grep
Resolve(path, tag string, refs []Range) Resolution
Commit(claim *Claim, next Grant)                // successful edit/write swap-in
ObserveWrite(path, tag string, anchors []string) // successful write
```

- `Resolve` answers `exact` (today's path), `rebased` (safety matrix below) or
  `refused{code}` — one struct, no boolean-plus-error dances at call sites.
- `Commit` is the transactional success half of today's Claim/Release pair:
  the old snapshot dies, the successor grant takes its place, atomically.
- Dispositions ring: the ledger remembers the last 8 `(path, tag, reason)`
  outcomes (consumed / evicted) so a retry against a dead TAG gets the precise
  code instead of a bare `no_capability`. This is where the historical
   one-string-hides-many-causes conflation dies.

### Typed outcomes

Every refusal carries a stable code and exactly one next step, rendered at the
tool boundary as:

```
[edit:<code>] <what happened>. Do not retry the same call unchanged. <next step>
```

| Code | Meaning | Next step in the message |
| --- | --- | --- |
| `no_capability` | No tracked editable observation of this path+TAG this session. | read with `mode:"edit"` (or editable grep), retry with the returned TAG and anchors. |
| `snapshot_consumed` | That snapshot's grants were consumed by an edit that applied; no successor grant covers the requested range. | Use the successor anchors from the last successful edit, or re-read `mode:"edit"`. |
| `snapshot_evicted` | The observation fell out of the bounded ledger. | Re-read `mode:"edit"`. |
| `anchor_not_observed` | Some endpoint LINE#HASH was never part of a grant of that snapshot (typo, hallucinated, wrong window). | Use exactly the anchors the read returned. |
| `mixed_grants` | Each endpoint is covered but no single grant covers a pair (two reads spliced). | Re-read the whole range in one read. |
| `ambiguous_reanchor` | Hash matches multiple candidate lines under the claimed shift. | Re-read `mode:"edit"` and use fresh anchors. |
| `tag_changed` | File on disk no longer hashes to the claimed TAG (external writer). | Re-read; the external change stands. |
| `changed_during_edit` | `unchangedTagGuard` tripped between read and swap. | Re-read and reapply onto the new content. |
| `invalid_ref` / `range_inverted` / `out_of_bounds` / `overlap` | Application-level, current texts kept, now typed. | Current guidance. |

Recovery ≤ 1 additional tool call is a success metric, not a promise: codes
exist so the model never needs a second *guess*.

## Re-anchoring safety matrix

Typical multi-edit failure: the model writes later ranges with line numbers
already shifted by its own earlier edit, while all ranges belong to one
original snapshot. `Resolve` treats the line number as a hint and the hash as
provenance. Re-anchoring applies only when **every** condition holds:

- same path and same full-file TAG as the claimed snapshot;
- exact `LINE#HASH` verification runs first — today's behavior is the fast path;
- each shifted endpoint's hash occurs at exactly one other line **inside the
  same grant** that observed it;
- all shifted endpoints across the whole `edits` array move by the same delta
  `d ≠ 0` (unshifted exact anchors may coexist);
- rebased ranges do not overlap each other or the exact ranges, and stay in
  bounds.

Any duplicate hash, differing deltas, a hash found only in a grant not claimed,
or overlap after rebase ⇒ typed refusal (`ambiguous_reanchor` /
`anchor_not_observed` / `overlap`). A successful rebase is *reported*:
`rebased edits[1] from 12-18 to 15-21 (delta +3)` — the model sees the
correction, it is never silent.

## Successor capability after edit

The harness knows the old revision, the transformation and the exact new
revision — the model should not re-read its own edit. On an applied edit,
`Commit` installs a grant for `(path, newTag)` covering the union of edited
regions ± 25 lines, capped at `maxGeneratedGrantAnchors = 512` anchors. The
edit result replaces *"Re-read this file before another edit"* with the new
TAG, the bounded anchor list (`maxDisplayedAnchors = 40` lines shown), and the
sentence that these anchors authorize the next edit of the shown range. A
failed edit still `Release`s — the old claim survives, unchanged behavior.

Old TAG dies with the commit: an external TAG change never mints a successor.

## Post-write capability

`WriteTool(ledger ...)` gains the ledger through assembly
(`internal/tools/tools.go` registry), no global. After a successful atomic
write, `ObserveWrite` installs a grant for the exact written revision —
anchors computed from the written content, capped at
`maxGeneratedGrantAnchors` from line 1; files longer than the cap need an
editable read for regions beyond it. The write result shows the new TAG and a
bounded anchor window with the same authorize-next-edit sentence. Failed or
canceled writes grant nothing.

This is a deliberate widening of the capability invariant — justified because
a capability is *a trusted tool result proving the model's knowledge of an
exact revision*, and the write result is exactly that. It widens no filesystem
permission: the permission gate still runs first, and write remains the
stronger operation.

## Plan-gate unique auto-binding

A wrong, missing or completed `plan_step` is bookkeeping, not intent. In
`Policy.Check`, before returning a step miss, compute candidates: steps of the
approved plan with status pending or in_progress whose type permits
`call.Name`.

- Exactly one candidate ⇒ bind it: `Verdict{StepID, StartPending}` plus a note
  naming the auto-bound id. A unique JIT candidate still raises the JIT demand
  — auto-binding never substitutes for the user grant.
- Several candidates ⇒ miss with a bounded candidate list (id, type, status;
  at most 8) — the model picks, the gate does not guess.
- Zero candidates ⇒ today's miss.

Unapproved plans stay denied, finished plans stay ungated, type ranks and the
permission gate are untouched, and numeric ordinals keep the legacy note.

## Telemetry and analyzer

Edit outcomes are recorded with a stable code (`exact`, `rebased`,
`refused/<code>`, plus `recovered` when a rebase or auto-bind avoided a
retry). `scripts/analyze_edit_errors.py` classifies new transcripts by code and
keeps the legacy text patterns for the historical corpus — old numbers stay
comparable, new numbers stop guessing.

| Metric | Baseline | Target |
| --- | --- | --- |
| `stale_anchors + no_capability` | 381 | −70% |
| blind retries (unchanged re-call) | 385 | −80% |
| recovery after typed refusal | — | ≤ 1 extra tool call |
| silent overwrites under concurrent modification | 0 | 0 (invariant, not a target) |

## Fail-closed never-list

Never auto-recover across: an external TAG change; duplicate or repeated
hashes; mixed grants; overlapping or post-rebase-overlapping ranges; anchors
never observed; paths outside the workspace; an unapproved or JIT-gated plan
step. Refusal is cheap; a wrong write is not.

## Task mapping

| Section | Task |
| --- | --- |
| Typed outcomes, module interface, dispositions ring | `typed-edit-capability-outcomes` |
| Re-anchoring matrix | `reanchor-shifted-edit-ranges` |
| Successor capability | `chain-edit-successor-capability` |
| Post-write capability | `authorize-post-write-edits` |
| Plan auto-binding | `auto-bind-unique-plan-step` |
| Telemetry, docs, baseline comparison | `verify-model-edit-reliability` |

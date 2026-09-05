# Edit reliability evaluation — 2026-09-05

The implementation and the measurement target have different outcomes.
The deterministic regressions cover the capability contract, including
session switching. The paired live run completed every fixture correctly on
both revisions, but cannot establish the epic's −70% stale/capability and
−80% unchanged-retry targets: those failures were already zero in the paired
baseline. The epic must not be marked complete on this evidence.

## Paired live protocol

- Baseline: `68fc3459a6b3f794ce0b00384c320e613ab22092`, immediately before
  the epic's first implementation commit.
- Current harness: `7d8513245aa49545c7c60bdabbfe05ad9843b356`.
- Configured models: `deepseek/deepseek-v4-flash` and
  `deepseek/deepseek-v4-pro`. These are two configured capacity tiers, not a
  claim that they represent every weak or strong model.
- Effort: provider default on both revisions. No effort override or
  provider-reported model version was available in these runs.
- Three identical two-turn fixtures, two independent runs per model and
  revision: incremental replacement; two distant replacements in a 140-line
  file followed by another replacement; create a file then edit it.
- Each run had a fresh temporary workspace and session, a 12-round limit per
  turn, and a 90-second deadline per turn. Runs for the two models were
  concurrent; fixtures within each model were sequential.
- Only built-in `read`, `grep`, `edit` and `write` were available, with
  workspace-only read/write permission. No shell, jobs, MCP, memory or project
  configuration was exposed. Requests for absent tools are still counted.
- Final expected file bytes, including an unchanged second file where
  applicable, were checked independently of the model's final answer.
  The current harness additionally checks unexpected files. This extra oracle
  and usage collection were added after the baseline; prompts and fixtures
  were unchanged. Neither revision's transcripts show a successful mutation
  outside the expected fixture files.

The opt-in runner is `internal/agent/edit_eval_test.go`. Its Go test result
describes whether evaluation execution succeeded; per-run `task_success`
records model correctness. A model failure must not disappear merely because
the evaluator itself completed. Normal tests skip all paid calls.

```sh
COZYPHI_EDIT_EVAL_MODELS=deepseek/deepseek-v4-flash,deepseek/deepseek-v4-pro \
COZYPHI_EDIT_EVAL_OUTPUT=/absolute/path/to/new-output \
COZYPHI_EDIT_EVAL_REVISION=<actual-harness-commit> \
go test ./internal/agent -run '^TestLiveEditReliability$' -v -count=1 -parallel=2

python3 scripts/analyze_edit_errors.py --root /absolute/path/to/new-output --json
```

The runner refuses to overwrite a populated run directory. Raw transcripts,
per-run manifests and JSON reports for this evaluation are local artifacts
under `.mcp-ai-helper/notes/edit-eval-20260905/`; no historical transcript was
modified. The same analyzer was used for both revisions.

## Observed outcomes

| Model | Correct tasks, before → after | Edit errors / attempts | Unchanged retries |
| --- | --- | --- | --- |
| Flash | 6/6 → 6/6 | 0/10 → 1/11 | 0 → 0 |
| Pro | 6/6 → 6/6 | 0/10 → 0/10 | 0 → 0 |

Current Flash had one `invalid_ref` edit and recovered after a trusted
observation. Neither model used a whole-file write after a refused edit.
The baseline made five `bash` requests and the current run made four; every
one was refused as tool-not-found, not executed.
Zero edit failures therefore does not mean zero model mistakes.

| Model | Editable reads, before → after | Grep requests | Tracked tool requests |
| --- | --- | --- | --- |
| Flash | 10 → 3 | 4 → 3 | 36 → 25 |
| Pro | 8 → 2 | 9 → 9 | 36 → 31 |

Editable reads exclude default view reads. Tracked requests include failed
and unavailable tool calls; request counts do not imply execution.

| Model | Reported total tokens, before → after | Sum of run elapsed times |
| --- | --- | --- |
| Flash | 312,536 → 244,542 | 73.215 s → 51.640 s |
| Pro | 325,226 → 299,563 | 101.524 s → 99.104 s |

Tokens include provider-reported prompt and completion usage across every
response, including responses without tool calls. Parallel tool calls do not
multiply response usage. This is token consumption, not a dollar cost: no
provider-reported price was recorded. Timing sums individual run durations,
not parallel wall-clock duration. Two repeats per scenario are too few to
generalize the observed token or latency differences.

## Deterministic safety evidence

These scripted tests prove behavior for specified inputs, not model quality:

- `internal/agent/edit_reliability_test.go`: real Engine/provider-adapter
  trajectory with malformed and unchanged retries, corrected exact edit,
  rebase, successor edit, write→edit, same-session compaction and fresh-engine
  resume refusing old anchors.
- `internal/agent/engine_session_switch_test.go`: implicit and explicitly
  supplied built-in tools retire grants on session switch; ordinary refresh
  preserves them; shared input tool slices do not share grants across engines;
  empty and unmarked custom toolsets preserve their intended behavior.
- `internal/tools/writetool/hashline_test.go`, `successor_test.go` and
  `internal/tools/editledger/ledger_test.go`: mixed exact/shifted batches,
  duplicate-hash ambiguity, mixed grants, overlap, short-TAG collision,
  rollback after refusal, long and distant ranges, generated/display budgets,
  and context-only truncation.
- `internal/atomicfile/pathlock_test.go` and writetool guard tests:
  cooperating writers serialize, observable intervening changes refuse, and
  workspace boundaries remain enforced.
- `internal/agent/executor_autobind_test.go` and plangate tests: unique binding
  succeeds; ambiguous binding, missing skill preload and JIT still gate work.
- `scripts/test_analyze_edit_errors.py`: 50 regressions separate corrected
  retries, semantic categories, chronological fallback, run correctness,
  path provenance, stable outcomes and response-level token attribution.

Arbitrary external writers still have a read-to-rename race window; the
precise guarantee is in [edit-capability.md](edit-capability.md). Short display
TAGs are not revision identity, and the full 64-bit fingerprint is not a
cryptographic authenticity proof.

## What remains unproven

The historical 227-transcript corpus had 500/2,905 edit failures and a legacy
"blind retries" count of 385. That counter included corrected retries, so it
cannot be compared directly with the new unchanged-arguments definition.
Raw failures from unequal workloads are not a valid reduction metric.

Establishing the percentage targets requires a prespecified matched workload
with enough baseline stale/capability failures and unchanged retries, larger
repeat counts, and reported uncertainty. These runs provide no estimate of
that reduction. Broad recovery from persistent model loops also depends on
the separate, unfinished `claude-stuck-detector` task. No harness can promise
that models of either tier stop making mistakes; safe refusal and measured
recovery remain the contract.

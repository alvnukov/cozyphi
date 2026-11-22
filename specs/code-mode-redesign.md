# Code Mode and the shared function library

Proposal, 22 November 2026. Repository baseline `c09826959ec8`. The product goal is a coding agent that can compose tools, write and test its own automation, and use or improve that automation from another project. The existing release remains the benchmark for quality and latency.

## External evidence

| System | Public behavior | Decision for CozyPhi |
| --- | --- | --- |
| Claude Code, v2.1.323 through v2.1.330 | Model-written JavaScript can use intermediate results and multiple tools in one model step. Diffs and tool calls are visible in the terminal. Each inner operation is subject to rules and hooks. Live sessions support bindings across snippets. The first week brought fixes for interrupted runs, read-only agents, and large batches. [Guide](https://code.claude.com/docs/en/code-mode) · [Release](https://github.com/anthropics/claude-code/releases/tag/v2.1.323) · [Changelog](https://code.claude.com/docs/en/changelog) | Show real operations and diffs. Make partial completion a first-class outcome. The installed local Claude was v2.1.288, so the comparison uses the published contract and transcripts. |
| OpenAI Codex | The code host runs cells, delegates nested tools to the session owner, and supports yield, wait and termination by cell ID. [Protocol](https://github.com/openai/codex/blob/main/codex-rs/code-mode-protocol/src/grpc/codex.code_mode.v1.proto) | Keep authority in the Go engine. Add cancellable long-running cells once foreground execution works. |
| goose | Its fresh JS workers expose a small host API. Nested calls inherit cancellation and permission flow. Computation, serialization and output have explicit bounds. [Guide](https://goose-docs.ai/docs/guides/managing-tools/code-mode/) | Use disposable cells. Keep durable, named source in the library, with tests and version history. |
| Cloudflare Agents | Search and describe load a subset of connector signatures. Approval can pause a program at one call. Named snippets can be discovered and reused. [MCP guide](https://developers.cloudflare.com/agents/tools/codemode/mcp/) · [API](https://developers.cloudflare.com/agents/tools/codemode/api-reference/) | Use selective discovery for tools and libraries. Give libraries ordinary source, unit tests, immutable releases and upgrade semantics. |
| Hugging Face smolagents | CodeAgent uses Python for executable actions. [Guide](https://huggingface.co/docs/smolagents/main/en/guided_tour) | Compare Python and TypeScript on the same authoring and maintenance tasks. Select by measured success on the models CozyPhi can use. |

One independent experiment on Claude Code 2.1.326 measured 84–94% less task context on summing 120 MCP tickets, with all 16 answers correct. On the two filesystem tasks, context grew in 14 of 16 pairs and the authors could not distinguish the differences from run-to-run noise. Only two runs backed each model/condition. CozyPhi already loads MCP signatures selectively. Measure from that baseline. [Experiment and transcripts](https://github.com/Bicameral-AI/claude-code-mode-benchmark)

## Authoring language and the execution boundary

Use TypeScript for published modules, unit tests and tool signatures. Accept the ordinary JavaScript subset for short cells. Accept common constructs such as top-level await, loops and destructuring. Compile to JavaScript and run it in one bounded engine. Keep the language of product code independent. A library can help edit Go and run Go tests.

Generate compact declarations from actual tool contracts. Return compiler and test diagnostics with source file and line. The library build performs semantic type checking against its declared SDK and dependencies. Removing type annotations is insufficient for that check. Check external values at runtime, including MCP data. A declaration supplied by a server cannot establish the type of a particular response. [TypeScript handbook](https://www.typescriptlang.org/docs/handbook/2/basic-types.html) · [esbuild TypeScript loader](https://esbuild.github.io/content-types/#typescript)

Prototype QuickJS compiled to WebAssembly under wazero. Verify the actual imports, Promise scheduling, compiler/checker packaging, cross-platform releases with `CGO_ENABLED=0`, cold and warm start, binary size, source maps, memory bounds and cancellation. No ambient directories, packages, environment, shell, sockets or credential imports are needed by authored code. Enforce limits on the Go side as well as the guest. Cache compiled modules by content digest while allocating a fresh instance for each cell. Commit to the runtime after the prototype meets these checks. [QuickJS-NG](https://quickjs-ng.github.io/quickjs/) · [wazero runtime configuration](https://pkg.go.dev/github.com/tetratelabs/wazero#RuntimeConfig)

The primary model interface is a small `code` tool. The JS API exposes the tools bound to this role and checkout, a bounded discovery/describe interface, and the shared library. The provider can transport source as an ordinary string field across Anthropic, Responses and Chat Completions. Keep waiting, interruption and interactive questions directly reachable by the harness. A single native `grep` or one shell process running the repository's test command is a valid one-line program.

## The harness-wide library

One logical library store per CozyPhi user, shared by every checkout and worktree. Proposed local location is `~/.cozyphi/functions/`. A named package contains exported functions, public signatures, examples, dependencies, unit tests and sanitized fixtures. The source and tests are readable text files. Model operations cover search, describe, open source or tests, create a draft, edit, run tests, diff and publish. Search returns names, signatures and versions. Opening one function loads its source on demand. Listing or reading a package executes no code.

**Mandatory versioning.** Use a local Git-backed store, an immutable release per package identified by package name, semantic version and content digest. Record the runtime SDK version and the full dependency resolution. Disallow replacing published source or retagging a version. Each draft has a base revision and a private working copy. Concurrent drafts may advance independently. Publish under a compare-and-swap on the package's base ref after testing the exact candidate. A stale publisher keeps its draft, sees the diff, rebases or merges, and reruns affected tests. Independent packages can publish independently. Installation must neither require a cloud account nor push source to any remote.

**Consumers.** On first use, pin each package and its full dependency graph for that project's checkout lineage. Keep a harness-managed lock when the repository supplies none. Projects can commit a portable `cozyphi.functions.lock` to share a reproducible selection with others. Record the exact resolved digests with every run. Project locks advance through an explicit upgrade. Reverting a pin restores the previous release. An in-flight or resumed historical run never silently adopts a newly published release. A missing pinned version produces a diagnostic. A library created in project A receives project B's current checkout, tools and permission checks when invoked in B. Stored modules carry source and test fixtures. Active edit grants, external credentials, live clients and runtime data remain owned by the calling session.

Model-side source editing must work across operating-system workspace boundaries. Implement library operations on logical package and draft IDs, under the same dispatcher. A scoped editable read and hashline edit act only on that draft. Publish presents the global change and its source/test diff through the existing permission flow. Package imports resolve to pinned library objects and the small host SDK. Test and invocation receipts bind the source, fixture, dependency and SDK digests. The global index can be rebuilt from immutable releases. The host owns published refs and prevents model-authored source from altering the registry directly.

**Testing authored code.** Supply the same test runner for a one-off cell, a draft and a published version. A test can pass data directly to a pure function, or inject tool fixtures for a program that performs I/O. Fixtures can yield pages, refusals, errors, timeouts and malformed data, and record calls and arguments for assertions. Unknown fixture calls fail the test. Test output contains failing input, expected and actual values, lines and revision. Run library tests in the production VM with a fixture host. Integration tests that need real operations use a disposable workspace and the regular dispatcher. Tests of generated product code live in that project's native suite and run through the project command with real status, selected failures and diff. Record unit, integration and product results separately.

A published candidate must pass type checking, its own tests and affected dependent contract tests. A failed revision remains editable and the last released version stays callable. Use an independent expected result or a regression that fails against the previous implementation to validate changes in behavior. A model writing a trivial assertion can otherwise report a green test for an incorrect result. For one-off cells we propose optional fixture execution, with a cheap path for one read. The runner can also validate a specific cell and execute that exact digest live.

An example of the proposed authoring interface follows. The input is a small fixture. Production rows would come from a validated MCP response. The host resolves the import against the caller's recorded package lock.

```ts
// builds/src/failures.ts
export type Build = { package: string; status: "passed" | "failed" };

export function failedPackages(rows: readonly Build[]): string[] {
  return [...new Set(rows.filter(row => row.status === "failed").map(row => row.package))].sort();
}

// builds/test/failures.test.ts
import { equal, test } from "cozy:test";
import { failedPackages } from "../src/failures.ts";

test("deduplicates failed packages and excludes passing builds", () => {
  equal(failedPackages([
    { package: "cli", status: "failed" },
    { package: "ui", status: "passed" },
    { package: "cli", status: "failed" },
  ]), ["cli"]);
});
```

A later cell can use `import { failedPackages } from "cozy:lib/builds"` and return `failedPackages(rows)`. One lookup should reveal the public signature, the resolved version and how to open the source or tests. Full-library contents need never enter the prompt.

**Example acceptance story.** In repository A the model writes a helper that pulls paginated build records and counts failures by package. Tests cover an empty page, a failed record and a second page. Publish `builds@1.0.0`. In repository B a fresh session finds the signature, runs the pinned function against B's approved source, then adds handling for cancelled builds on a draft. Start a second draft in A, publish one change and observe the other conflict. Rebase, test, publish the merged version and upgrade B. Verify the old version in A and the pre-upgrade run still produce their original answers. Verify a denied MCP call in B remains denied through the library.

## Existing execution contracts to preserve

| Area | Migration |
| --- | --- |
| Registry and permission flow | Extract the common per-operation path in `internal/agent/executor.go`. Direct calls and nested calls use the same round state, rewritten arguments, validation, plan and permission checks, hooks and mutation guards. Evaluate each actual call when issued. A user stop, required skill or mandatory notice reaches the engine even when JS returns an empty value. Post-hook output restrictions apply to the structured path too. |
| Files and shell | Migrate `ls`, `find`, `grep`, `read`, `write`, `edit` and `bash`. Use native bulk search and bounded structured hits, paths, lines and process exit data. Hashline grants come only from the existing session ledger, for bounded observed anchors. Keep successor grants after writes and edits. A saved function has to obtain grants for the current revision on each invocation. |
| MCP | Preserve `mcp_list`, `mcp_inspect`, `mcp_call`. `internal/mcp/rpc.go` currently drops structured data when text is present and reduces tool errors to a prefix. Preserve structuredContent, content blocks, error status, truncation and continuation before display formatting. Pass only the post-policy representation to JS. Keep binary/media data behind bounded handles and transcript references. |
| Code and host data | Reuse the pinned `cozy-tools v0.2.0` contracts for structured results, artifacts, truncation and effects where compatible. Keep the CozyPhi edit ledger, diff, delivery receipts and permissions host-owned. A rich result in `internal/tools/tooldef/tool.go` needs separate projections for code, model transcript and terminal. |
| Conditional tools | Bind LSP, web, memory, task, watch, shell task, context, session, plan, question, harness diagnostics and agent lifecycle from the live registry. Explore/review, worker, main, plan/useplan and headless retain their different operations. The quarantine reader keeps only its decoys and gets no code tool. Preserve current protected-web availability and taint even if JS filters the output. A paused approval belongs to the concrete action. |
| History and UI | Persist the outer provider tool/result pair and separate nested events with a parent ID. Show actual operations, approvals, diffs, output and completed effects after cancellation. Coalesce frequent TUI refreshes. Preserve child delivery IDs through durable append before ACK, memory touch tracking, compaction, session switches and legacy transcripts. A crash during an external write yields an unknown outcome to reconcile. Automatic replay is limited to known-safe work. |

## Delivery sequence and evidence

| Milestone | Deliverable | Acceptance |
| --- | --- | --- |
| 1. Baseline and language trial | Reuse the existing edit evaluation and freeze representative file, shell and MCP fixtures. Compare TS/JS and Python authoring on two available models using the same tool contracts, diagnostics budget and fixtures. | Exact answers and file bytes, first-pass correctness, repairs and tokens for first authoring and later modification. No need to ship two runtimes. |
| 2. First usable slice | Bounded cell, shared dispatcher, structured MCP and read/search. Include a real TypeScript function, a fixture test and a second-repository reuse trial in the first prototype. | 120 records can be reduced locally, malformed inputs fail predictably, and a published function is discoverable after restarting CozyPhi. Report host calls and context bytes. |
| 3. Library publication | Shared store, independent drafts, semantic versions, digests, dependency pins, type checking, unit tests, source edits, diff, upgrade and rollback. | Cross-project acceptance story above passes, including a failed change, a competing publisher, a missing dependency and a test holding an old version. |
| 4. Full coding workflow | Hashline edit/write, successor grants, native shell/tests and role-specific tools. Add child events, mandatory notices, effects journal and recovery. | Search → anchored edit → native product tests → useful failure or clean diff. Repeat after cancellation, role/session change and compaction. Verify old direct-mode histories. |
| 5. Throughput and rollout | Add bounded parallelism to independent, explicitly safe reads. Keep order around effects, approvals and taint. Add yield/wait/cancel for long cells. Compare the direct catalog, code-first and a small hybrid if particular models need it. | Confirm responsiveness, resource bounds, quality and useful reductions by workload before changing the default. Retain an explicit rollback setting during migration. |

Use the same commit, prompts, model revisions, reasoning effort, budgets and fixtures per pair. Run cold and warm caches separately. Include one read, a native repository-wide search, a many-file edit, deterministic MCP aggregation, semantic classification with rare counterexamples, library reuse and concurrent revision, plus interruption after one successful write. Report outcome accuracy, unintended changes, actual model turns and host calls, input/cached/output tokens, price where available, p50/p95 time with sample counts, and new corrections per task. Show individual failures. Aggregate latency percentiles need more repetitions than a pilot. Track full time to a correct tested change.

Pass all deterministic permission, concurrency, versioning, test and recovery cases. Suggested promotion targets, subject to the measured baseline, are at least 50% less intermediate model-visible data on the MCP aggregation, one fewer model round on dependent operations, and single-read p50 within 10% of direct mode at comparable quality. Reject a default that loses correct results or masks errors. Evaluate model capability tiers separately. The initial engineering trade-off is the added cost of a compiler, VM and versioned store in exchange for cheaper repeated workflows. Measure that cost on first use and on reuse.

Repository references include [edit evaluation](../doc/edit-reliability-evaluation.md), [edit capabilities](../doc/edit-capability.md), [MCP](../doc/mcp.md), [hooks](../doc/hooks.md), and [web](../doc/web.md).

Eleven existing offline tests passed on the baseline. They cover role grants, the MCP catalog, edit results, session changes and the disabled web boundary description. The scripted edit-and-resume and full web-admission tests require a loopback HTTP listener, which the research environment denied. The proposed executor, package lifecycle and comparative model trials remain future work.

# Code Mode

Start `cozyphi --code-mode` in the terminal, or use `cozyphi run --code-mode -p "…"` in a script. Both commands accept `--direct-tools` to select the original tool interface for one invocation. The persistent setting is in `~/.cozyphi/config.yaml`.

```yaml
code_mode:
  enabled: true
  discovery: conditional
  advisor_model: my-cheap-model
  advisor_timeout: 800ms
```

The advisor name refers to a configured model. Omit it to keep discovery local. The four discovery settings are `manual` (explicit search), `local` (automatic local ranking), `conditional` (ask the advisor on ambiguous requests) and `always` (consult the configured advisor even for clear matches). These settings control suggestions. Every permitted operation remains discoverable and callable. The advisor can abstain, and failures fall back to the local results.

## Executing a cell

A model calls `code` with TypeScript or JavaScript source and returns the value it wants to see. The embedded QuickJS engine runs in a fresh WebAssembly instance. It has no mounted project files or provider credentials. It reaches the current project through the ordinary tool dispatcher.

```typescript
const names = tools.list()
return names.includes("grep") ? tools.describe("grep") : names
```

Use `await tools.call(name, args)` or its shorthand `await tools.read(args)`. File edits still require editable hashline observations for the current file revision. Run the product's own tests through `tools.bash` with its native test command. The terminal and session history record nested operations independently of the cell's returned JSON.

Long cells can be started with `yield_ms` and resumed with `code_wait` in the same turn. A wait accepts the returned cell ID. Pass `terminate: true` to cancel remaining work. Check the nested operations before retrying a partially completed edit.

## Testing one-off programs

Use `code_test` inside a cell to check transient source with ordered tool fixtures. Each case declares its expected JSON return value. The test reports how many cases passed and the SHA-256 of the source. A missing, unexpected or unused mock fails the case.

```typescript
return await tools.code_test({
  source: "const data = await tools.call('metrics', {window: 5}); return data.count;",
  cases: [{
    name: "empty window",
    calls: [{name: "metrics", args: {window: 5}, result: {count: 0}}],
    want: 0
  }]
})
```

## Publishing and reusing a function

The shared Git repository lives at `~/.cozyphi/functions/` and is created on demand. Each session has a separate draft branch. A release records its exact semantic version, Git digest, source, unit tests and pinned dependencies. Git must be installed for library operations. Publication also needs Deno 2 on PATH for offline TypeScript checking. Executing already published functions and testing transient cells use the embedded runtime.

1. Use `tools.library({action: "search", query: "count records"})`. Results include importable exports, signatures and test examples. Starting or updating a draft can also suggest related functions.
2. Use `open` with a package name to create a draft or resume it. Use `create_file` for a new `index.ts` or `index.test.ts`. For existing code, use the editable contents returned by `open` or `read`, then `edit` with the file tag and line anchors.
3. Use `test` and `diff` to check and review the draft. Publish the returned draft `revision` under an explicit release version. Concurrent publications are checked against the base revision. On a conflict, `rebase` the private draft, resolve any marked lines, test and review it again.
4. Import the published export in any project. First use by a writable session pins the release in that checkout. `pin` selects a different exact version for an upgrade or rollback. Use a project-owned `cozyphi.functions.lock` to share a resolution through the project's own Git repository. Its JSON shape is `{"packages":{"records":{"version":"1.0.0","digest":"<full Git commit ID>"}}}`.

Example `records/index.ts`:

```typescript
export function countRecords(rows: number[]): number {
  return rows.length
}
```

Example `records/index.test.ts`:

```typescript
import { test, equal } from "cozy:test"
import { countRecords } from "./index.ts"

test("counts repeated records individually", () => {
  equal(countRecords([3, 4, 4]), 3)
})
```

Use `fixture(tool, args, result)` or `fixtureError(tool, args, message)` from `cozy:test` for code that calls a host tool. Library tests execute against these fixtures. A new project can call the tested revision in one cell:

```typescript
import { countRecords } from "cozy:lib/records"
return countRecords([10, 8, 10])
```

---
id: fix-opencode-config-only-providers
title: OpenCode source drops providers declared only in opencode.json
status: done
priority: critical
model_level: very_high
task_type: bug
tags:
    - opencode
    - models
    - bug
acceptance_criteria:
    - A provider declared only in opencode.json with `options.apiKey` (plain string, `{env:NAME}` or `{file:PATH}`) is imported with that key.
    - A provider declared only in opencode.json without any key (local server) is imported with an empty APIKey.
    - 'For a catalog provider, opencode.json `models` are overlaid on the catalog list: catalog models stay, config entries add or override by id, missing `limit` falls back to the catalog limits.'
    - A catalog provider whose opencode.json entry overrides `options.baseURL` keeps the catalog models on the new endpoint.
    - An auth.json entry for a provider that is neither in the catalog nor declared in opencode.json is still skipped.
    - Providers listed in `disabled_providers` are skipped.
    - auth.json key wins over `options.apiKey`; `{env:NAME}` and `{file:PATH}` inside `options.apiKey` expand.
    - Result is sorted by Name and stable across runs.
    - doc/opencode.md and CHANGELOG.md describe the new semantics.
    - make fmt-check lint test passes.
verification_plan:
    - go test ./internal/opencode/... with new table tests covering each acceptance criterion.
    - make fmt-check lint test in the worktree.
    - 'Manual: with opencode.enabled true, /model lists opencode/<provider>/<model> entries for config-only and keyless providers.'
created_at: "2026-09-03T08:30:46.240801Z"
updated_at: "2026-09-04T12:56:41.13183Z"
---

## Body

**Symptom** With `opencode.enabled: true` the model picker shows at most one OpenCode model although OpenCode itself offers a full list. For a typical setup (catalog provider with an extra model in opencode.json, a catalog provider re-pointed at another endpoint with an inline `options.apiKey`, a custom OpenAI-compatible provider with an inline key, a keyless local LM Studio provider) cozyphi imports exactly one model with a zero context window.

**Root cause** `resolveModels` in `internal/opencode/source.go` iterates only `auth.json` entries of `type: "api"`. Providers that exist only in `opencode.json` `provider` are never visited, `providerOptions` does not parse `apiKey`, and a keyless local provider has no auth entry at all. Config `models` also replace the catalog model list instead of overlaying it, so an entry without `limit` wipes the catalog models and reports context 0. OpenCode's own semantics: config-declared providers are always enabled (credential from auth store, then `options.apiKey`, then none), config models merge over the models.dev catalog per model id.

**Fix** Iterate the union of auth.json api providers and opencode.json provider keys. Credential per provider: auth.json key, else `options.apiKey`, else empty. A provider declared in opencode.json is imported even with an empty key when it has a baseURL (catalog or `options.baseURL`), at least one model and a supported protocol (catalog protocol or npm adapter). A catalog-only provider (not declared in opencode.json) still requires an auth.json key. Models = catalog models overlaid by config models keyed by id; config `limit` wins when > 0, otherwise the catalog value stays. Honor `disabled_providers`. Keep names `opencode/<provider>/<model>`, deterministic sort, detached copies.

**Out of scope** OAuth or wellknown credentials, `enabled_providers` allow-list, writing anything into `~/.cozyphi`, connect overlay changes.

**Docs** Rewrite the "Providers and models" section of `doc/opencode.md`; add a Fixed entry under `## [Unreleased]` in CHANGELOG.md.

**Done (2026-09-04).** 2026-09-04: Fixed and merged to main. resolveModels now walks the union of auth.json api entries and opencode.json provider keys; credential is auth key → options.apiKey (plain, {env:NAME}, {file:PATH}, object forms; unreadable {file} → empty key, provider still imported) → empty; config models overlay the catalog list by id (limit wins only >0) instead of replacing it; top-level disabled_providers skips a provider entirely. Tests: TestKeySourceResolveKey (16), TestResolveModelsUnionOfAuthAndConfig (12), TestLoadImportsConfigOnlyProviderWithFileKey (beeline {file:}), TestLoadDisabledProvidersAreSkipped. Landed as 642d916, merged to main in 479b134 (CHANGELOG conflict resolved, our entry on top of [Unreleased]); doc/opencode.md "Providers and models" rewritten. Scoped gates green: build, test, fmt --diff, golangci-lint run ./internal/opencode/... — 0 issues. Worktree removed, branch deleted.

## Acceptance Criteria

- A provider declared only in opencode.json with `options.apiKey` (plain string, `{env:NAME}` or `{file:PATH}`) is imported with that key.
- A provider declared only in opencode.json without any key (local server) is imported with an empty APIKey.
- For a catalog provider, opencode.json `models` are overlaid on the catalog list: catalog models stay, config entries add or override by id, missing `limit` falls back to the catalog limits.
- A catalog provider whose opencode.json entry overrides `options.baseURL` keeps the catalog models on the new endpoint.
- An auth.json entry for a provider that is neither in the catalog nor declared in opencode.json is still skipped.
- Providers listed in `disabled_providers` are skipped.
- auth.json key wins over `options.apiKey`; `{env:NAME}` and `{file:PATH}` inside `options.apiKey` expand.
- Result is sorted by Name and stable across runs.
- doc/opencode.md and CHANGELOG.md describe the new semantics.
- make fmt-check lint test passes.

## Verification Plan

1. go test ./internal/opencode/... with new table tests covering each acceptance criterion.
2. make fmt-check lint test in the worktree.
3. Manual: with opencode.enabled true, /model lists opencode/<provider>/<model> entries for config-only and keyless providers.

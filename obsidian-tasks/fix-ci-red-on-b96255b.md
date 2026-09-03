---
id: fix-ci-red-on-b96255b
title: 'Fix CI red on b96255b: windows legs + fmt-check drift'
status: done
tags:
    - ci
    - windows
    - memory
created_at: "2026-08-30T22:09:55.517967Z"
updated_at: "2026-08-30T22:09:55.517967Z"
---

## Body

CI run on b96255b failed two jobs.

- test (windows): TestLoopSeesAMemoryRewrittenInPlace — real bug: memoryTouched compared raw JSON arguments (doubled backslashes) against the memory dir, so Windows never invalidated the store on in-place rewrite; also mangled recall queries. Fixed by callNamesDir/decodeToolPath with boundary-aware escaped-twin matching + unit test (commit efd013e).
- test (windows): TestConfigHandlerMasksAPIKeysAndWritesOwnerOnly asserted POSIX 0600 perms; Windows Stat reports 0666. Guarded the assert on runtime.GOOS (commit 964f7b4).
- fmt-check: golangci v2.13.0 gofumpt flags xui/clipboard.go and xui/render/render.go; local PATH had 2.12.2 so make fmt-check passed locally. Reformatted with the pinned version (commit b6117bf).
- lint (local go-installed binary only): 4 revive unhandled-error on cleanup-path Close calls; made them explicit (commit cf88a4a).

Verified: full local gate on v2.13.0 (fmt-check, lint 0 issues, test, test-race) green.

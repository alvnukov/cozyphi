---
id: fix-perm-allow-all-config-corruption
title: 'fix: allow-all toggle corrupts config.yaml written with inline permissions'
status: done
task_type: bug
verification_plan:
    - go test ./internal/project/ — new SetDangerouslyAllowAll tests green
    - go vet ./..., golangci-lint clean
    - real ~/.phi/config.yaml parses and phi starts
created_at: "2026-08-24T12:00:46.792972Z"
updated_at: "2026-08-24T12:06:25.038968Z"
---

## Body

Repro: config.yaml saved by `phi config` carries `permissions: {}` (empty inline section from marshaling an untouched permDoc). Choosing "allow all for every session" in the TUI permission dialog calls project.SetDangerouslyAllowAll (controller.go:378), which copies the `permissions: {}` line verbatim, marks the block open, then appends `  dangerously_allow_all: true` — an indented child under an inline mapping is invalid YAML. Every subsequent start fails: "parse ~/.phi/config.yaml: yaml: line 8: did not find expected key".

Fix in SetDangerouslyAllowAll: normalize `permissions: {}` to block form `permissions:` before appending the child; refuse a non-empty inline mapping (`permissions: {…}`) with an actionable error instead of corrupting the file. Tests: exact-output regression on the user's file shape, LoadConfig round-trip (the property that burned the user), replace-existing, append-when-missing, inline-refusal.

## Verification Plan

1. go test ./internal/project/ — new SetDangerouslyAllowAll tests green
2. go vet ./..., golangci-lint clean
3. real ~/.phi/config.yaml parses and phi starts

---
id: lint-install-official-binary
title: make lint-install should install the official golangci-lint binary, not go-install it
status: todo
tags:
    - ci
    - tooling
verification_plan:
    - make lint-install reports the official binary
    - golangci-lint version matches .golangci-lint-version
    - make lint and CI lint agree on the same tree
created_at: "2026-08-30T22:09:59.859565Z"
updated_at: "2026-08-30T22:09:59.859565Z"
---

## Body

The go-installed v2.13.0 and the official v2.13.0 release binary disagree: official reports 0 issues on b96255b, go-installed flags 4 revive unhandled-error findings. Same tag, same config, different binary — the local gate then checks something CI does not run (and vice versa: the fmt-check drift on xui/ slipped through the same way).

Switch make lint-install to the official install script (golangci-lint-action uses the release binary), keyed off .golangci-lint-version, so local lint/fmt-check run the exact bytes CI runs. Add a smoke check that golangci-lint version matches the pin.

## Verification Plan

1. make lint-install reports the official binary
2. golangci-lint version matches .golangci-lint-version
3. make lint and CI lint agree on the same tree

---
id: make-lsp-action-schema-tolerate-generated-defaults
title: Make lsp action schema tolerate generated defaults
status: done
tags:
    - issue
    - feedback
created_at: "2026-08-26T20:37:37.889886Z"
updated_at: "2026-08-26T21:12:59.727989Z"
---

## Body

Built-in `lsp` uses one action-based schema whose operation-specific fields may be flattened into required arguments by the tool client. A diagnostics call was emitted with default navigation fields (`include_declaration`, `direction`, etc.), then rejected because those fields only apply to other operations. Repro: invoke lsp diagnostics through a client that fills all declared fields. Expected: read-only diagnostics succeeds or tolerates known irrelevant generated defaults; strict unknown-field validation remains.

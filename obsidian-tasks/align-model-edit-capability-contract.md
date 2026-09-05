---
id: align-model-edit-capability-contract
title: Align model-facing instructions with successor edit capabilities
status: done
priority: high
model_level: medium
task_type: bug
parent_id: reliable-model-file-edits
tags:
    - review
    - prompt
    - reliability
acceptance_criteria:
    - Read, edit, write descriptions, system prompt and context-loading documentation agree on read→edit→edit, write→edit and failed-edit recovery.
    - The wording distinguishes shown successor ranges from ranges needing an editable read.
    - A public tool-definition/prompt check catches reintroduction of the contradictory lifecycle instructions.
verification_plan:
    - Inspect assembled model prompt plus tool definitions, not only internal success strings.
    - Run focused prompt/tool-contract tests.
created_at: "2026-09-05T06:42:03.970533Z"
updated_at: "2026-09-05T07:55:12.943262Z"
---

## Body

Confirmed on main 0032d62. editDescription in internal/tools/writetool/hashline.go:25-33 still says a successful edit ends authorization and requires a re-read. internal/agent/prompt/system-prompt.tmpl:37 instructs read before edit. Actual success output grants the next edit without re-reading, and writeDescription advertises this. readDescription says one edit attempt while failed attempts retain authorization. doc/context-loading.md:17 also says failures consume it. These conflicting instructions can manufacture rereads/protocol mistakes; model-dependent frequency has not been measured. Align all model-facing contract surfaces and docs with the implemented lifecycle; do not weaken gates.

**Done (2026-09-05, commit e438ce7, merged into main):** the `read`, `edit`, `write` and `grep` descriptions, the system prompt `# Change` section, doc/context-loading.md and AGENTS.md now agree on the implemented lifecycle: a failed edit keeps its authorization, a successful `edit` or `write` prints the TAG and anchors that authorize the next edit without a re-read, and only a line whose anchor was not printed needs a fresh `read` with `mode:"edit"`. New `internal/tools/contract_test.go` assembles the real tool descriptions and the built system prompt and fails on the old "ends the authorization" / "one-shot" wording while requiring the successor wording, so the contradiction cannot be reintroduced silently. CHANGELOG Fixed entry added. Gates on main after merge: build, `go test ./internal/tools/... ./internal/agent/prompt/...` green.

## Acceptance Criteria

- Read, edit, write descriptions, system prompt and context-loading documentation agree on read→edit→edit, write→edit and failed-edit recovery.
- The wording distinguishes shown successor ranges from ranges needing an editable read.
- A public tool-definition/prompt check catches reintroduction of the contradictory lifecycle instructions.

## Verification Plan

1. Inspect assembled model prompt plus tool definitions, not only internal success strings.
2. Run focused prompt/tool-contract tests.

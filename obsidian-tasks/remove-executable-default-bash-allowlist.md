---
id: remove-executable-default-bash-allowlist
title: Remove code-executing commands from default Bash auto-allow
status: done
priority: critical
model_level: high
task_type: bug
parent_id: harness-security-hardening
tags:
    - security
    - permissions
    - rce
    - prompt-injection
acceptance_criteria:
    - Commands that execute repository-controlled code never receive default Allow.
    - Approval is parameter-bound and shown after complete command parsing.
    - Writes to execution-control files such as .git/config and hook directories require explicit approval.
    - Read-only commands retained in the allowlist have adversarial parsing tests.
verification_plan:
    - Table-driven tests cover go test/build/generate, compound syntax, config-write then command chains, and readonly modes.
created_at: "2026-08-24T13:20:17.833178Z"
updated_at: "2026-09-20T12:56:47.504942Z"
---

## Body

internal/permission/defaults.go auto-allows go test and go build. Repository-controlled tests/build inputs can execute code, so indirect prompt injection can combine an allowed workspace write with an automatically allowed test/build and reach host RCE without a fresh approval. Classify commands by effects; executable build/test/package lifecycle commands must Ask by default. Protect harness and VCS control files that can alter later execution.

**Done (2026-09-20).** Delivered on main by 82354f73 (go build/test/vet/fmt/mod ask by default; only read-only go commands left allowlisted), b8bf4277 (allowlist binds to the single simple command; control syntax, unclosed quotes, metacharacters leave the allowlist — fail closed), 9e8810ae (writes to .git/config, hooks/, linked-worktree admin ask; symlinked leaves resolved). Verified 2026-09-20: go test ./internal/permission ok; criteria mapped to TestDefaultGoCommandsAskByDefault, TestUserAllowOptsIntoGoTest, TestBashAllowlistBindsToTheFullCommand, TestWriteGitControlFilesAsk, TestWriteGitControlFilesAskAcrossWorktrees, TestPolicyControlPathAskOverridesDerivation. Adjacent finding filed separately as bash-allowlist-sensitive-read.

## Acceptance Criteria

- Commands that execute repository-controlled code never receive default Allow.
- Approval is parameter-bound and shown after complete command parsing.
- Writes to execution-control files such as .git/config and hook directories require explicit approval.
- Read-only commands retained in the allowlist have adversarial parsing tests.

## Verification Plan

1. Table-driven tests cover go test/build/generate, compound syntax, config-write then command chains, and readonly modes.

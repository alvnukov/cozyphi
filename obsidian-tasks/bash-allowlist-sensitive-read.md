---
id: bash-allowlist-sensitive-read
title: Allowlisted read-only bash bypasses the sensitive-path deny
status: todo
priority: high
model_level: high
task_type: bug
parent_id: harness-security-hardening
tags:
    - security
    - permissions
    - secrets
acceptance_criteria:
    - A simple allowlisted command that names a sensitive path (literal /etc/shadow, ~/, $HOME/ forms) never receives default Allow; it asks
    - Ambiguous or unresolvable arguments fail closed to Ask without breaking ordinary workspace reads (cat main.go stays Allow)
    - Adversarial tests cover quoting, ~ and $HOME forms, symlinked arguments resolving into a sensitive prefix, and head/tail/wc variants
verification_plan:
    - 'Table-driven tests in internal/permission: each retained read-only allowlist entry against sensitive-path arguments asks; workspace arguments stay Allow'
    - 'Manual: cat of the user''s own key file in the TUI opens an ask, cat of a workspace file does not'
created_at: "2026-09-20T12:55:09.249752Z"
updated_at: "2026-09-20T12:55:09.249752Z"
---

## Body

**Repro (proven 2026-09-20, DefaultPolicy, scratch test on main).** `cat /etc/shadow`, `cat ~/.ssh/id_ed25519`, `head -c 100 /etc/shadow`, `tail ~/.ssh/known_hosts` all return Allow from `StaticGate.Check`.

**Mechanism.** `checkBash` (internal/permission/gate.go) judges syntax only: a simple command matching `^cat\b` / `^head\b` / `^tail\b` (internal/permission/defaults.go) is auto-allowed with no path semantics. The sensitive-path deny (`SensitivePathDeny` → `checkPaths`) applies to the read family (read/grep/find/ls) but never to bash arguments, so allowlisted bash is a bypass of the very boundary the read tool enforces.

**Impact.** Indirect prompt injection gets user secrets (~/.ssh keys, .aws/credentials, .gnupg, ~/.cozyphi/config.yaml, /etc/shadow) into the transcript without a single approval; combined with any egress channel this is exfiltration. No code executes, which is why this is not part of remove-executable-default-bash-allowlist (that ticket closed as delivered).

**Fix shape to evaluate.** When an allowlisted command's arguments are path-shaped, resolve them like `checkPaths` does (~ and $HOME expansion, ResolveTarget for symlinked leaves) and ask on a sensitive-prefix hit; unparseable arguments fail closed to Ask. Must not break `cat main.go` in the workspace or `git log --grep='a;b'`-style quoting.

## Acceptance Criteria

- A simple allowlisted command that names a sensitive path (literal /etc/shadow, ~/, $HOME/ forms) never receives default Allow; it asks
- Ambiguous or unresolvable arguments fail closed to Ask without breaking ordinary workspace reads (cat main.go stays Allow)
- Adversarial tests cover quoting, ~ and $HOME forms, symlinked arguments resolving into a sensitive prefix, and head/tail/wc variants

## Verification Plan

1. Table-driven tests in internal/permission: each retained read-only allowlist entry against sensitive-path arguments asks; workspace arguments stay Allow
2. Manual: cat of the user's own key file in the TUI opens an ask, cat of a workspace file does not

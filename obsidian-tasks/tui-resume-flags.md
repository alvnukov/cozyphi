---
id: tui-resume-flags
title: 'CLI: open TUI directly at a session (--resume <id>, --continue/-c)'
status: done
priority: high
task_type: feature
tags:
    - ux
    - phase1
    - cli
acceptance_criteria:
    - '`phi --continue` (or -c) opens the TUI with the newest session for the cwd loaded'
    - '`phi --resume <id-or-prefix>` opens that session; ambiguous prefix errors with candidates'
    - With no sessions, --continue fails with an actionable message and exit code 3
    - TUI first frame already shows resumed history + session id
    - '`phi` bare behavior unchanged (new session)'
verification_plan:
    - unit tests for flag parsing incl. mutual exclusivity
    - integration-ish test of controller wiring (fake engine or httptest)
    - 'manual: phi --continue, phi --resume <prefix>, ambiguous prefix'
created_at: "2026-08-23T17:18:11.217511Z"
updated_at: "2026-08-23T17:34:20.53148Z"
---

## Body

User need: from the shell, open a specific session or the latest one in the TUI — like `claude --resume <id>` / `claude -c`. Today the TUI has no startup resume flags at all: `phi` always starts a new session, and resume is only reachable inside via /sessions + /resume <id>. The only CLI surface with resume plumbing is `phi run --session/--continue-last`, which is headless one-shot and demands -p — wrong tool for "open a session".

Plan: parse flags after `phi`/`phi tui` (or accept `phi resume [<id>]`); --resume takes id or unique prefix (Controller.Resume already supports prefix), --continue/-c picks list[0] via session.ListSessions; pass into the editor/controller before the first frame so the transcript renders the resumed history immediately; no prompt required. Keep cmd assembly lean per AGENTS.md (constructors take parameters).

## Acceptance Criteria

- `phi --continue` (or -c) opens the TUI with the newest session for the cwd loaded
- `phi --resume <id-or-prefix>` opens that session; ambiguous prefix errors with candidates
- With no sessions, --continue fails with an actionable message and exit code 3
- TUI first frame already shows resumed history + session id
- `phi` bare behavior unchanged (new session)

## Verification Plan

1. unit tests for flag parsing incl. mutual exclusivity
2. integration-ish test of controller wiring (fake engine or httptest)
3. manual: phi --continue, phi --resume <prefix>, ambiguous prefix

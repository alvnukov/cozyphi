---
id: run-prompt-ux
title: 'phi run: positional prompt, stdin support, friendlier missing-prompt hint'
status: todo
priority: medium
task_type: feature
tags:
    - ux
    - phase1
    - cli
acceptance_criteria:
    - positional args after flags become the prompt
    - piped stdin is appended to -p prompt (or used as prompt when -p absent)
    - 'missing prompt with tty stdin: one-line error hinting at `phi` for interactive use'
    - usage dump only on -h/--help or unknown flag
verification_plan:
    - table tests for parseRunArgs with positional args
    - stdin pipe test via runLoop harness
    - 'manual: phi run ''hi'', echo x | phi run -p ''wrap'', phi run'
created_at: "2026-08-23T17:18:11.220852Z"
updated_at: "2026-08-23T17:18:11.220852Z"
---

## Body

`phi run` without -p bails with 'prompt is required' + full usage dump. The guard itself is right for a headless one-shot (loop needs an initial user turn), but the ergonomics lag claude/opencode:

- positional prompt: `phi run fix the bug` should work like `opencode run "..."` (currently: 'unknown flag "fix"');
- stdin: `echo log | phi run -p "explain"` should append piped stdin to the prompt (claude-style); `phi run` with piped stdin and no -p could take stdin as the prompt;
- missing-prompt error should be one line and point interactive users to `phi` (TUI) instead of dumping 20 lines of usage.

Deprioritized vs tui-resume-flags: that one covers the real use case this complaint came from.

## Acceptance Criteria

- positional args after flags become the prompt
- piped stdin is appended to -p prompt (or used as prompt when -p absent)
- missing prompt with tty stdin: one-line error hinting at `phi` for interactive use
- usage dump only on -h/--help or unknown flag

## Verification Plan

1. table tests for parseRunArgs with positional args
2. stdin pipe test via runLoop harness
3. manual: phi run 'hi', echo x | phi run -p 'wrap', phi run

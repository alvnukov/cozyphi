---
id: editor-esc-recall-flake
title: Flaky TestEditorEscRecallsQueuedPrompt on slow runners
status: todo
priority: medium
task_type: bug
verification_plan:
    - 'Reproduce: run the single test with -count=50 under artificial load (e.g. stress) or with GOMAXPROCS=1'
    - 'Fix: make the assertion wait on the submit-state transition instead of assuming it'
    - 'Verify: -count=50 green locally; CI green on both OS legs twice'
created_at: "2026-09-04T22:26:36.868044Z"
updated_at: "2026-09-04T22:26:36.868044Z"
---

## Body

**What**: `TestEditorEscRecallsQueuedPrompt` (`internal/tui/editor/editor_queue_test.go:440`) fails intermittently in CI with "input must be submittable again", assertion Should be true.

**Evidence (2026-09-04, PR #2 checks, run 33925253453 — diff had zero .go changes)**:
- coverage (ubuntu): FAIL at 0.14s
- test (macos-latest): FAIL at 0.29s
- test (ubuntu-latest): PASS same code
- local (fast machine): 20/20 PASS via `go test ./internal/tui/editor/ -run TestEditorEscRecallsQueuedPrompt -count=20`

**Read**: timing-sensitive — the test asserts submit state after ESC recall; on slow/loaded runners (macos, coverage instrumentation) the editor state machine has not reached "submittable" when asserted. Likely needs an eventual-consistency wait/pump loop instead of a fixed-step assertion.

**Scope**: one test in internal/tui/editor. CI noise blocks honest branch protection later — required checks flaking red is the worst case for enforcement.

## Verification Plan

1. Reproduce: run the single test with -count=50 under artificial load (e.g. stress) or with GOMAXPROCS=1
2. Fix: make the assertion wait on the submit-state transition instead of assuming it
3. Verify: -count=50 green locally; CI green on both OS legs twice

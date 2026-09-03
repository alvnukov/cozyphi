---
id: fix-composer-fakebus-race
title: Data race in composer test fakeBus.Publish between mention-search goroutines
status: done
priority: high
model_level: medium
task_type: bug
tags:
    - tui
    - composer
    - tests
    - race
created_at: "2026-08-25T22:34:55.042791Z"
updated_at: "2026-08-30T20:52:51.945299Z"
---

## Body

go test -race ./internal/tui/composer intermittently reports a data race: two scheduleMentionSearch goroutines call (*fakeBus).Publish concurrently (composer_test.go:19 via pane.go:630, seen in TestComposerMentionOffersAgents / TestChatInputStartsSingleLine). Observed twice in full-suite runs during LSP task verification. Serialize the fake (mutex) or debounce the goroutines so the test fake is race-free.

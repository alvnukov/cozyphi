---
id: voice-pause-focus-loss
title: 'Voice: switching windows resets a paused microphone'
status: in_progress
priority: high
task_type: bug
branch: bug/voice-pause-focus-loss
worktree_path: .worktrees/voice-pause-focus-loss
acceptance_criteria:
    - 'Тап Space — toggle: нажатие флипает listening↔paused, отпускание (реальное или синтетическое при потере фокуса) микрофон никогда не флипает'
    - Потеря фокуса терминала очищает только spaceDown — пауза стоит (TestLosingFocusNeverFlipsTheMicrophone)
    - 'HoldKeys-поверхность удалена: Options.HoldKeys, View.VoiceHoldKeys, «hold keys» в /voice status, доки и тесты'
    - Регрессионные тесты в internal/tui/composer/voice_test.go зелёные вместе с sessions и voice
verification_plan:
    - go test ./internal/tui/composer/
    - golangci-lint run ./internal/tui/composer/ (один раз перед коммитом)
    - 'Ручная проверка: тап-пауза + Cmd-Tab → пауза стоит'
created_at: "2026-09-08T11:41:26.563433Z"
updated_at: "2026-09-08T12:01:31.210449Z"
---

## Body

**Bug.** Голосовой режим включён и стоит на паузе; переключение на другое окно
сбрасывает паузу — микрофон снова начинает слушать.

**Root cause** — `internal/tui/composer/pane.go`, ветка `xui.FocusEvent`:
при потере фокуса вызывается `releaseSpace()`, если `spaceDown`. `releaseSpace`
флипает микрофон, когда `now - spacePressedAt >= 300ms`. В терминале, который
не доставляет key release (`releasesSeen == false`), `spaceDown` остаётся true
навсегда после тапа Space (флип тапа уже произошёл на нажатии, release не
придёт). Синтетический release при потере фокуса видит elapsed >> 300ms →
`flipVoice()` → Paused → Listening + VoiceResume. Пауза сброшена.

Где release-события доставляются — тап снимает `spaceDown` сразу, и семантика
hold-to-talk при потере фокуса (release ушёл в другое окно) корректна и должна
сохраниться.

**Fix:** в ветке FocusEvent применять release-семантику только при
`c.releasesSeen`; иначе просто очищать `spaceDown` без флипа.

## Acceptance Criteria

- Тап Space — toggle: нажатие флипает listening↔paused, отпускание (реальное или синтетическое при потере фокуса) микрофон никогда не флипает
- Потеря фокуса терминала очищает только spaceDown — пауза стоит (TestLosingFocusNeverFlipsTheMicrophone)
- HoldKeys-поверхность удалена: Options.HoldKeys, View.VoiceHoldKeys, «hold keys» в /voice status, доки и тесты
- Регрессионные тесты в internal/tui/composer/voice_test.go зелёные вместе с sessions и voice

## Verification Plan

1. go test ./internal/tui/composer/
2. golangci-lint run ./internal/tui/composer/ (один раз перед коммитом)
3. Ручная проверка: тап-пауза + Cmd-Tab → пауза стоит

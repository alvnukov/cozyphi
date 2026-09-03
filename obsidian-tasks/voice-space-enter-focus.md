---
id: voice-space-enter-focus
title: 'Voice dialog mode: Space and Enter never reach the composer while the chat input is focused'
status: in_progress
priority: critical
model_level: high
task_type: bug
tags:
    - voice
    - bug
    - tui
    - composer
acceptance_criteria:
    - In voice dialog mode a plain Space (no modifiers) pauses/resumes the microphone through the real dispatch order (focused chat input first, then the pane) and no space character is typed
    - In voice dialog mode a plain Enter closes the segment and waits for the queue (submitVoice) instead of the chat input submitting directly; Shift/Alt/Ctrl+Enter still insert a newline
    - Outside the mode, with a picker open, or during reverse-i-search Space and Enter behave exactly as before
    - chat.ChatInput gains an exported VoiceMode flag mirrored by the composer from the voice state; unit tests cover the flag and the composer voice tests deliver keys through the focused-widget order
    - CHANGELOG bullet at the top of Unreleased; specs/voice-dialog.md mentions the chat input deferral; make fmt-check lint test passes
verification_plan:
    - go test ./internal/components/chat/ ./internal/tui/composer/
    - make fmt-check lint test in the task worktree, GATE rc=0
    - 'Manual: cozyphi, Ctrl+G, speak, tap Space → hint row shows ‖ paused and no space appears in the composer; tap again → ● listening; Enter → ⋯ finishing… then send'
created_at: "2026-09-03T21:17:40.602916Z"
updated_at: "2026-09-03T21:19:49.615541Z"
---

## Body

**Симптом.** В голосовом режиме (Ctrl+G) нажатие пробела печатает пробел в поле ввода, пауза/возобновление микрофона не происходит. Enter отправляет сообщение сразу, не дожидаясь очереди распознавания.

**Причина.** `App.dispatch` (internal/components/app/app.go) отдаёт клавишу сначала сфокусированному виджету — это `chat.ChatInput` — и, если тот её потребил, панель композера событие не видит. `ChatInput.Handle` вставляет любую руну ≥ 0x20 (пробел в том числе) и на голый Enter вызывает `OnSubmit`, потребляя событие. Поэтому `ComposerPane.handleVoiceKey` (internal/tui/composer/voice.go) до пробела и Enter не доходит. Тесты фазы 2 вызывали `c.Handle(&components.EventContext{}, ev)` напрямую, минуя фокусный порядок, и дефект не поймали. Отпускания клавиш `ChatInput` не потребляет (`if !e.Press { return }`), так что `releaseSpace` работает.

**Исправление.** По образцу `MentionOpen`/`SlashOpen`: у `chat.ChatInput` появляется экспортируемое поле `VoiceMode bool` — «выставляется композером, пока включён голосовой режим; голый Space и голый Enter остаются непотреблёнными, чтобы их обработало правило голосового режима; всё остальное печатается как обычно». В `Handle`: в ветке `KeyEnter` — `if c.completerOpen() || (c.VoiceMode && e.Mods == 0) { return }` (Shift/Alt/Ctrl+Enter по-прежнему вставляют перевод строки); в ветке `KeyRune` — `if c.VoiceMode && e.Rune == ' ' && e.Mods == 0 { return }` перед вставкой. Reverse-i-search обрабатывается раньше и остаётся как есть (в поиске пробел — часть запроса). Композер зеркалит флаг: `c.Chat.VoiceMode = c.voiceState != voice.StateIdle` в `ApplyVoiceState` и `resetVoice` (общий маленький хелпер).

**Тесты.** В `internal/components/chat` — тест: при `VoiceMode` голый Space и Enter не потребляются и текст не меняется, Shift+Enter вставляет `\n`, буква печатается; без флага пробел печатается. В `internal/tui/composer/voice_test.go` хелпер `send` доставляет событие в реальном порядке: `ctx := &components.EventContext{}; c.Chat.Handle(ctx, ev); if ctx.Consume { return }; ctx.DeliveredTo = &c.Chat; c.Handle(ctx, ev)` — вся существующая матрица (tap/hold/repeat/пикеры/модификаторы) должна остаться зелёной; добавить кейсы: режим включён → Space не печатается и состояние переключилось; Enter при Listening → `voiceSubmitPending`, `OnSubmit` не вызван; режим выключен → пробел печатается; слэш-пикер открыт → пробел печатается.

**Документация.** CHANGELOG (вверху Unreleased): `- Voice: Space and Enter reach the dialog mode again — the focused chat input used to type the space and send the message before the mode saw them.` В specs/voice-dialog.md в разделе реализации — абзац про `ChatInput.VoiceMode` и порядок доставки событий.

**Ветка/worktree.** `bug/voice-space-enter-focus`, `.worktrees/voice-space-enter-focus`. Коммит: `fix(voice): let space and enter reach the dialog mode`.

## Acceptance Criteria

- In voice dialog mode a plain Space (no modifiers) pauses/resumes the microphone through the real dispatch order (focused chat input first, then the pane) and no space character is typed
- In voice dialog mode a plain Enter closes the segment and waits for the queue (submitVoice) instead of the chat input submitting directly; Shift/Alt/Ctrl+Enter still insert a newline
- Outside the mode, with a picker open, or during reverse-i-search Space and Enter behave exactly as before
- chat.ChatInput gains an exported VoiceMode flag mirrored by the composer from the voice state; unit tests cover the flag and the composer voice tests deliver keys through the focused-widget order
- CHANGELOG bullet at the top of Unreleased; specs/voice-dialog.md mentions the chat input deferral; make fmt-check lint test passes

## Verification Plan

1. go test ./internal/components/chat/ ./internal/tui/composer/
2. make fmt-check lint test in the task worktree, GATE rc=0
3. Manual: cozyphi, Ctrl+G, speak, tap Space → hint row shows ‖ paused and no space appears in the composer; tap again → ● listening; Enter → ⋯ finishing… then send

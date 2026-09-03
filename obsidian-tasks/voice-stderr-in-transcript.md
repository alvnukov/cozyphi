---
id: voice-stderr-in-transcript
title: 'Voice: whisper-cli stderr log lands in the composer as the transcript'
status: in_progress
priority: critical
model_level: high
task_type: bug
tags:
    - voice
    - bug
    - proc
acceptance_criteria:
    - Транскрипт командного STT берётся только из stdout процесса; stderr в поле ввода не попадает
    - При ненулевом exit code ошибка показывает последнюю содержательную строку stderr, а не первую строку лога
    - Ограничение объёма вывода сохраняется отдельно для stdout и хвоста stderr
    - Тест воспроизводит сценарий whisper-cli (лог в stderr, текст в stdout) и проверяет чистый транскрипт
    - make fmt-check lint test зелёный, CHANGELOG дополнен
verification_plan:
    - go test ./internal/voice/... ./internal/proc/...
    - make fmt-check lint test в воркtree с логом и GATE rc=
    - 'Ручная проверка: Ctrl+G с whisper-cli 1.9.2 (Metal) — в поле ввода только распознанный текст'
created_at: "2026-09-03T20:58:09.319819Z"
updated_at: "2026-09-03T20:58:15.513452Z"
---

## Body

**Симптом.** Пользователь говорит в голосовом режиме, а в поле ввода появляется лог whisper-cli (ggml_metal_init, system_info, auto-detected language, whisper_print_timings) вместе с распознанным текстом. Голосовой ввод фактически не работает при установленном whisper.

**Причина.** `internal/proc/proc.go` `Run` направляет `cmd.Stdout` и `cmd.Stderr` в один sink, а `internal/voice/stt_command.go` `Transcribe` отдаёт `res.Output` целиком в `NormalizeTranscript`. whisper-cli пишет все логи в stderr, транскрипт в stdout.

**Исправление.** В `internal/proc` добавить вариант запуска с раздельным захватом: stdout полностью (в пределах лимита), stderr как ограниченный хвост (`DefaultStderrLimit`), без изменения поведения существующего `Run` для других вызывающих. Командный транскрайбер использует stdout как транскрипт; при ненулевом exit code в сообщение об ошибке идёт последняя непустая строка stderr после `redact.Redact`, при пустом stderr последняя строка stdout. Тест в `stt_command_test.go`: `sh -c 'echo log >&2; echo " hello "'` даёт `hello`; тест ошибки проверяет строку из stderr.

**Не трогать.** Захват аудио (`capture.go`) уже читает stdout отдельно и хранит хвост stderr, менять его не нужно. HTTP-транскрайбер не затронут.

**Связано.** Задача `voice-model-install` (фича установки моделей) идёт параллельно в другом воркtree и не касается `internal/proc` и `stt_command.go`.

## Acceptance Criteria

- Транскрипт командного STT берётся только из stdout процесса; stderr в поле ввода не попадает
- При ненулевом exit code ошибка показывает последнюю содержательную строку stderr, а не первую строку лога
- Ограничение объёма вывода сохраняется отдельно для stdout и хвоста stderr
- Тест воспроизводит сценарий whisper-cli (лог в stderr, текст в stdout) и проверяет чистый транскрипт
- make fmt-check lint test зелёный, CHANGELOG дополнен

## Verification Plan

1. go test ./internal/voice/... ./internal/proc/...
2. make fmt-check lint test в воркtree с логом и GATE rc=
3. Ручная проверка: Ctrl+G с whisper-cli 1.9.2 (Metal) — в поле ввода только распознанный текст

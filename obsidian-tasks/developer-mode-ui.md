---
id: developer-mode-ui
title: 14 — Показать UI, notifications и voice без побочных действий
status: done
priority: medium
model_level: medium
task_type: feature
parent_id: developer-mode-readonly
tags:
    - developer-mode
acceptance_criteria:
    - 'ui category покрывает theme/keybindings/notifications/voice: configured, loaded, реально применённое значение и apply semantics.'
    - TUI-состояние читается безопасно через владельца без data races; headless сохраняет сведения о configuration, а UI runtime поля имеют not_applicable.
    - Read не включает microphone/capture, не отправляет notifications и не меняет theme/keybindings/UI preferences.
    - Нет raw device/provider credentials, пользовательских notification payloads и небезопасных path/error strings.
    - В catalog явно названы поддержанные настройки и исключения; snapshot не делает render/activation нужным для наблюдения.
verification_plan:
    - Table tests configured/loaded/applied keybind/theme/notification/voice и restart semantics.
    - 'Одинаковая конфигурация в TUI/headless: совпадают config layers, runtime not_applicable обоснован.'
    - Spies capture/send/render/write не вызываются collector; профильный race-test передачи status.
created_at: "2026-09-06T09:09:20.782128Z"
updated_at: "2026-09-07T00:33:03.367664Z"
---

## Body

**Сделано.** Категория `ui` из 15 полей в четырёх пространствах — `surface`, `theme`, `keybinds`/`keymap`, `notify`, `voice` (`internal/diag/ui.go` и файлы `ui_theme.go`, `ui_keys.go`, `ui_notify.go`, `ui_voice.go`). Проекции пишут владельцы: `components.ObserveTheme`, `keys.ObserveConfig`/`Observe`, `notify.ObserveConfig` и `(*Notifier).Observe`, `voice.ObserveConfig`/`Observe`. Живая половина не читается из tool goroutine: `View` отдаёт detached-снимок через `Controller.PublishUIStatus` на своей goroutine, а вопрос читает только сохранённый снимок и его `Revision`. Headless (`cmd/run.go`, `headlessUIFacts`) отвечает `not_applicable` на runtime-слои и при этом сохраняет все configured-значения. Диалект `editmode` берётся из preferences один раз при сборке сессии — ответ файл не перечитывает. `uiReason` переписан под бюджет ответа (508 байт при `MaxValueBytes` 512), чтобы перечень исключений не срезался bounder-ом.

**Тесты.** `internal/diag/ui_test.go` — три слоя отвечают на три вопроса, таблица apply/scope на 15 полей, headless ничего не прячет, неопубликованная поверхность — `unavailable`, а не отсутствие, таблицы delivery и `VoiceMissing`, свип «ни одно значение не несёт `/`, `\`, `+`, ctrl, alt, http», catalog с проверкой длины reason. Пакетные тесты наблюдения: `internal/tui/keys/observe_test.go` (смена диалекта двигает defaults, ребайнд в тот же аккорд — accepted, но не diverged), `internal/notify/observe_test.go` (сломанный sender при mode=always), `internal/components/observe_test.go` (каждый builtin резолвится через `ThemeByName`), `internal/voice/observe_test.go` (`CaptureGate` занят другой сессией; ключ/команда/устройство/endpoint не путешествуют), `internal/tui/sessions/observe_test.go` (публикация ничего не шлёт: `turns == 0`). `internal/tui/controller/developer_mode_ui_test.go` — опубликованное и есть прочитанное, `-race` на 200 публикациях против 200 снимков, персистентный диалект отличается от «никто не сохранял». `cmd/run_developer_ui_test.go` — headless-ответ без `F9`, `Ctrl+`, cwd и путей.

**Gates.** `gofmt -l` чисто по всем изменённым файлам; `golangci-lint run` по 8 затронутым деревьям — 0 issues; `go test` по ним же — ok; `go test -race -run PublishingAndObserving ./internal/tui/controller/` — ok. После merge в main перепроверены авто-смёрженные пакеты `internal/components` и `internal/tui/sessions` — ok.

**Коммиты.** `5d92ead feat(diag): observe the surface a session runs behind`, merge `f4a70af`.

## Acceptance Criteria

- ui category покрывает theme/keybindings/notifications/voice: configured, loaded, реально применённое значение и apply semantics.
- TUI-состояние читается безопасно через владельца без data races; headless сохраняет сведения о configuration, а UI runtime поля имеют not_applicable.
- Read не включает microphone/capture, не отправляет notifications и не меняет theme/keybindings/UI preferences.
- Нет raw device/provider credentials, пользовательских notification payloads и небезопасных path/error strings.
- В catalog явно названы поддержанные настройки и исключения; snapshot не делает render/activation нужным для наблюдения.

## Verification Plan

1. Table tests configured/loaded/applied keybind/theme/notification/voice и restart semantics.
2. Одинаковая конфигурация в TUI/headless: совпадают config layers, runtime not_applicable обоснован.
3. Spies capture/send/render/write не вызываются collector; профильный race-test передачи status.

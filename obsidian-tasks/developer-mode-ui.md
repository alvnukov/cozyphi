---
id: developer-mode-ui
title: 14 — Показать UI, notifications и voice без побочных действий
status: blocked
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
updated_at: "2026-09-06T09:09:20.782128Z"
---

## Body

**Что построить.** Категория UI-настроек и runtime с явными отличиями TUI/headless. Читать epic developer-mode-readonly.

**Blocked by:** developer-mode-tui.

**Фиксированные решения.** UI остаётся render-only потребителем, module диагностики не зависит от widgets. Передавать detached status владельца через существующий lifecycle/синхронизацию, не читать UI maps из фонового tool goroutine. Не требовать нового developer экрана или изменений /status. Headless not_applicable относится к runtime UI, а не скрывает существующую configured настройку.

**Работа.** Task worktree; unit/controller fixtures без реального терминала, звука/уведомлений; closeout по epic.

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

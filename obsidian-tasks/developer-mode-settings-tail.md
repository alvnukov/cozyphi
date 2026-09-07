---
id: developer-mode-settings-tail
title: Досказать хвост настроек, который harness view пропускает
status: todo
priority: low
model_level: medium
task_type: feature
parent_id: developer-mode-readonly
tags:
    - developer-mode
acceptance_criteria:
    - Каждая из перечисленных настроек сопоставлена с полем каталога либо явно обоснована как невыводимая; инвентаризация больше не держит их в missing.
    - Подсказки, глоссарий и пути журналов отчитываются счётчиками и фактом источника, без содержимого и без абсолютных путей.
    - Потолки headless-запуска видны в headless и честно отсутствуют в TUI, а не показывают нули.
    - Scoped-прогон только затронутых пакетов; тесты владельцев обновлены вместе с developer-mode тестами.
verification_plan:
    - go test ./internal/voice/ ./internal/diag/ ./internal/tui/controller/ и headless-сценарии в cmd.
    - Проверить, что TestEverySettingTheHarnessAcceptsIsAccountedFor и TestEveryGapNamesATicketThatBlocksTheEpic проходят после переноса записей.
    - Прогнать sentinel-тест с посаженным секретом в voice.hints и в пути журнала.
created_at: "2026-09-07T02:03:53.815203Z"
updated_at: "2026-09-07T02:03:53.815203Z"
---

## Body

**Что не так.** Пятнадцать настроек меняют поведение сессии и не отражены ни одним полем каталога. Поодиночке каждая мелкая, вместе они дыра в обещании полноты: модель не может увидеть, почему запись голоса обрывается, почему headless-запуск остановился, и что процесс ведёт второй журнал. Найдено при инвентаризации в developer-mode-coverage; сейчас записано там как gap с этим id.

**Config.** `internal/voice:FileConfig.language`, `.auto_pause_seconds`, `.max_seconds`, `.segment_silence_ms`, `.hints`, `.glossary`; `internal/voice:STTFileConfig.provider`, `.timeout_seconds`; `internal/project:UIState.stopLimitDisabled`. Подсказки и глоссарий пишет пользователь — отчитываться счётчиком, не содержимым.

**ENV.** `COZYPHI_MCP_LOG_DIR`, `COZYPHI_PLAN_GATE_LOG_DIR` — включают отдельные журналы подсистем. Не видно ни включённости, ни назначения. Путь наружу не выводить: хватит факта «журнал включён из окружения» рядом с diagnostics.logging.

**CLI.** `--jsonl`, `--max-rounds`, `--timeout` — форма вывода и потолки headless-запуска. `runtime.mode` уже говорит headless, но не говорит, под какими ограничениями.

**Что сделать.** Голос — расширить `voice.Observe`, три-четыре поля в категории ui. Журналы — рядом с diagnostics.logging, как факт и источник. Флаги headless — в runtime или diagnostics вместе с harness.limits. Каждое поле объявить в каталоге и перенести запись из `missingConfig`/`missing` в `reported` в `internal/tui/controller/developer_mode_coverage_test.go`.

## Acceptance Criteria

- Каждая из перечисленных настроек сопоставлена с полем каталога либо явно обоснована как невыводимая; инвентаризация больше не держит их в missing.
- Подсказки, глоссарий и пути журналов отчитываются счётчиками и фактом источника, без содержимого и без абсолютных путей.
- Потолки headless-запуска видны в headless и честно отсутствуют в TUI, а не показывают нули.
- Scoped-прогон только затронутых пакетов; тесты владельцев обновлены вместе с developer-mode тестами.

## Verification Plan

1. go test ./internal/voice/ ./internal/diag/ ./internal/tui/controller/ и headless-сценарии в cmd.
2. Проверить, что TestEverySettingTheHarnessAcceptsIsAccountedFor и TestEveryGapNamesATicketThatBlocksTheEpic проходят после переноса записей.
3. Прогнать sentinel-тест с посаженным секретом в voice.hints и в пути журнала.

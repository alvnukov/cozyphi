---
id: developer-mode-web-policy
title: Показать политику web-инструментов в harness view
status: done
priority: medium
model_level: medium
task_type: feature
parent_id: developer-mode-readonly
tags:
    - developer-mode
acceptance_criteria:
    - Каждое поле секции web сопоставлено с полем каталога либо явно обосновано как невыводимое; инвентаризация в developer-mode-coverage больше не держит их в missing.
    - Учётные данные и адреса поиска отчитываются как наличие/вид/источник; ни значение, ни хэш, ни суффикс не попадают в ответ, ошибку, transcript и audit.
    - Категория отвечает в обеих точках входа (TUI и headless) одним контрактом, наблюдение не открывает соединений и не трогает кэш.
    - Есть тесты владельца и developer-mode тесты; scoped-прогон только затронутых пакетов.
verification_plan:
    - go test на пакете-владельце web, internal/diag и internal/tui/controller; отдельно headless-сценарии в cmd.
    - Прогнать TestNoSecretReachesAnyAnswerRefusalTranscriptOrRecord с посаженным sentinel в google_api_key и search_url.
    - Убедиться, что TestEverySettingTheHarnessAcceptsIsAccountedFor проходит после переноса записей из missing в reported.
created_at: "2026-09-07T02:03:04.969483Z"
updated_at: "2026-09-07T06:50:29.605121Z"
---

## Body

**Что не так.** Секция `web` в config.yaml — двадцать настроек, определяющих, выходит ли процесс в сеть и куда именно, — не представлена в каталоге ни одним полем. Модель в developer mode может прочитать про модель, права, планы и хранилища, но про единственную подсистему, которая открывает исходящие соединения, не может прочитать ничего. Найдено при инвентаризации в developer-mode-coverage; сейчас записано там как gap с этим id.

**Что именно не покрыто.** `internal/project:fileConfig.web` и все поля `internal/project:webFileConfig.*`: enabled, allow, allowed_hosts, denied_hosts, allowed_schemes, accepted_content_types, cache_dir, google_api_key, google_api_key_env, google_cse_id, google_cse_url, max_redirects, max_search_results, max_source_bytes, quarantine, search_provider, search_url, timeout_seconds, user_agent.

**Секреты.** `google_api_key` — учётные данные, `google_api_key_env` — имя переменной, `search_url`/`google_cse_url` могут нести токен в query. Отчитываться по правилам эпика: наличие и источник, без значений, хэшей и суффиксов; хосты и схемы — счётчиками и видом политики, не списком, если список пишет пользователь.

**Что сделать.** Решить, где живёт субъект: отдельная категория или подраздел permissions рядом с bash/mcp (egress — это граница, а не настройка UI). Написать owner-проекцию по образцу `internal/permission`/`internal/voice`: пакет-владелец отдаёт facts, collector их только раскладывает. Подключить в `newDiagnostics` и в headless-обвязку, объявить ключи в каталоге, добавить тесты владельца и developer-mode тесты обеих точек входа. После этого перенести записи из `missingConfig` в `reportedConfig` в `internal/tui/controller/developer_mode_coverage_test.go` — тест инвентаризации не даст забыть.

## Acceptance Criteria

- Каждое поле секции web сопоставлено с полем каталога либо явно обосновано как невыводимое; инвентаризация в developer-mode-coverage больше не держит их в missing.
- Учётные данные и адреса поиска отчитываются как наличие/вид/источник; ни значение, ни хэш, ни суффикс не попадают в ответ, ошибку, transcript и audit.
- Категория отвечает в обеих точках входа (TUI и headless) одним контрактом, наблюдение не открывает соединений и не трогает кэш.
- Есть тесты владельца и developer-mode тесты; scoped-прогон только затронутых пакетов.

## Verification Plan

1. go test на пакете-владельце web, internal/diag и internal/tui/controller; отдельно headless-сценарии в cmd.
2. Прогнать TestNoSecretReachesAnyAnswerRefusalTranscriptOrRecord с посаженным sentinel в google_api_key и search_url.
3. Убедиться, что TestEverySettingTheHarnessAcceptsIsAccountedFor проходит после переноса записей из missing в reported.

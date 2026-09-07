---
id: developer-mode-docs
title: 17 — Документировать проверенный read-only developer mode
status: done
priority: medium
model_level: low
task_type: docs
parent_id: developer-mode-readonly
tags:
    - developer-mode
acceptance_criteria:
    - Документация показывает реальные cozyphi/cozyphi tui/cozyphi run --developer-mode формы и catalog/snapshot/explain примеры, проверенные на fixtures.
    - Описаны все категории, configured/loaded/effective, provenance, unset/redacted/unavailable/not_applicable, freshness/revisions и apply semantics.
    - Явно описаны полностью read-only поведение, CLI-only активация, отсутствие наследования детьми и обычный permission gate; нет обещания sandbox или специального developer consent.
    - Не документируются write/reload/reset, developer launcher или ещё не реализованные поля; исключения согласованы с coverage matrix.
    - Обновлены релевантные пользовательские ссылки и Unreleased без изменения released sections; нет старого имени продукта phi.
    - Примеры не содержат настоящих credentials/переписки; изменения только документационные.
verification_plan:
    - Сопоставить каждый пример и описанное поле с CLI help/tool schema и coverage matrix; только fixture значения.
    - Проверить ссылки и changelog diff, released section не затронут.
    - Проверить запреты и терминологию cozyphi; не запускать общерепозиторные Go gates ради docs.
created_at: "2026-09-06T09:10:07.752844Z"
updated_at: "2026-09-07T02:19:31.461742Z"
---

## Body

**Что построить.** Пользовательское руководство по реализованному read-only developer mode и актуальные help/reference примеры. Читать epic developer-mode-readonly и завершённую coverage matrix.

**Blocked by:** developer-mode-coverage.

**Scope.** Документация и changelog, без изменения runtime и без перепроектирования interface. Сохранять язык конкретных существующих документов и английские CLI/служебные identifiers; task notes вести по-русски. Не расширять эту задачу до массовой очистки branding: remove-legacy-phi-branding отдельная задача. Если нужны изменения agent instructions, сначала применить writing-for-agents; в нормальном scope достаточно пользовательской документации.

**Работа.** Task worktree; проверки ссылок, примеров/help и diff, без Go gates всего repo. Commit/merge/ledger/cleanup по epic; push только по просьбе.

**Сделано.** Коммит 8e8e415, слит в main. Новый `doc/developer-mode.md` (371 строка), ссылка из README и запись в Unreleased. Изменения только документационные: ни одного файла в `internal/` или `cmd/`.

Документ идёт от включения к содержимому. Четыре формы запуска взяты из справки самих команд (`cmd/tui.go`, `cmd/run.go`); отдельным списком — почему флаг единственный способ: ни config, ни ENV, ни slash-команда, ни возобновление сессии, ни дочерний агент. Приведён текст отказа для вызова без capability. Прямо сказано, что это не sandbox и не отдельное согласие: инструмент стоит за обычным permission gate.

Дальше три действия и одиннадцать категорий с однострочным описанием каждой; четыре значения availability, причём `not_implemented` помечен как состояние, в котором сегодня нет ни одной категории — это то же утверждение, что проверяет тест из билета 16. Три слоя, лестница источников (`default` → `config_file` → `env` → `session` → `plan`, а `cli_flag`/`computed`/`build`/`unknown` вне её), пять состояний, таблицы `apply` и `scope` по значениям, `observed_at`/`freshness`/`revision`.

**Примеры.** Все сняты с fixture-сессии (один модель kestrel, `compact_threshold: 150000`, `notifications.mode: unfocused`, ничего не отрисовано) временным дамп-тестом, который удалён до коммита. Один `explain` приведён ровно так, как его отдаёт инструмент, — чтобы было видно, что каждый член value сериализуется всегда и `false` не читается как отсутствие. Дальше расхождение слоёв (`notify.mode`), слой, которого у поля нет (`notify.delivery`, `not_applicable`), отсутствующий инструмент как факт о workspace (`tool.task`), отказ на несуществующий ключ. Границы — из живого поля `diagnostics.harness.limits`, и настоящий overview этой сессии действительно `truncated: true` с просьбой сузить: paging нет.

**Согласовано с матрицей полноты.** Раздел «что не показывается» отделяет навсегда удержанное (геометрия панелей, командная строка захвата и устройство, `PATH`, переменные пути импорта opencode, адреса провайдеров) от того, у чего пока нет поля (вся политика `web`, часть голосовых настроек, `COZYPHI_MCP_LOG_DIR`, `COZYPHI_PLAN_GATE_LOG_DIR`, `--jsonl`/`--max-rounds`/`--timeout`) — те же две колонки, что в инвентаризации билета 16. Ни одно нереализованное поле не описано как существующее.

**Аудит.** Показана настоящая строка записи и сказано, что она пишется только при включённом `COZYPHI_DEBUG` и что в ней нет места значению, источнику, причине или тексту ошибки; четыре исхода названы.

**Проверки.** Все JSON-примеры разобраны парсером — 6 из 6. Ссылка `hooks.md` существует. Диффа ниже `## [0.20.0]` нет: единственная правка CHANGELOG — вставка 17 строк на позиции 11, внутри Unreleased. Старого имени продукта в изменённых местах нет (единственные вхождения «phi run» — в released-секции 0.19.0, их трогает отдельный тикет remove-legacy-phi-branding). Go-гейты не запускались: изменений в коде нет.

## Acceptance Criteria

- Документация показывает реальные cozyphi/cozyphi tui/cozyphi run --developer-mode формы и catalog/snapshot/explain примеры, проверенные на fixtures.
- Описаны все категории, configured/loaded/effective, provenance, unset/redacted/unavailable/not_applicable, freshness/revisions и apply semantics.
- Явно описаны полностью read-only поведение, CLI-only активация, отсутствие наследования детьми и обычный permission gate; нет обещания sandbox или специального developer consent.
- Не документируются write/reload/reset, developer launcher или ещё не реализованные поля; исключения согласованы с coverage matrix.
- Обновлены релевантные пользовательские ссылки и Unreleased без изменения released sections; нет старого имени продукта phi.
- Примеры не содержат настоящих credentials/переписки; изменения только документационные.

## Verification Plan

1. Сопоставить каждый пример и описанное поле с CLI help/tool schema и coverage matrix; только fixture значения.
2. Проверить ссылки и changelog diff, released section не затронут.
3. Проверить запреты и терминологию cozyphi; не запускать общерепозиторные Go gates ради docs.

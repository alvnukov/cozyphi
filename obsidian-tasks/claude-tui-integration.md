---
id: claude-tui-integration
title: 'TUI: @claude в композере, /claude status|verify|threads, строки job с ценой, раздел в settings'
status: todo
priority: medium
model_level: medium
task_type: feature
parent_id: claude-consult-tool
tags:
    - claude
    - tui
    - composer
    - settings
acceptance_criteria:
    - '@claude и @claude:mode превращаются в вызов тулы с verbatim task; /claude status|threads|verify работают и не создают user message.'
    - Строка claude-job показывает режим, модель, живой прогресс и футер с ценой; usage-pane показывает claude.
    - Settings показывает статус и режимы; make fmt-check lint test rc=0.
verification_plan:
    - go test ./internal/agent/... ./internal/tui/... -run 'Claude' -race.
    - Ручной прогон в TUI всех трёх слэшей и упоминания.
    - make fmt-check lint test.
created_at: "2026-09-02T05:13:31.875482Z"
updated_at: "2026-09-02T05:13:31.875482Z"
---

## Body

**Цель.** Пользователь зовёт Claude напрямую и видит, что это стоило.

**Композер.** @claude[:mode] <задача> в delegation.go рядом с @explore/@worker/@review: инструкция модели вызвать claude с этим режимом (дефолт architect) и текстом verbatim как task, затем передать ответ; пункт в пикере упоминаний.

**Слэш.** /claude status — op=status в локальную строку; /claude threads — реестр тредов; /claude verify [вопрос] — последний ответ ассистента как вложение «answer under review» + вопрос → mode=architect, результат как локальная строка транскрипта и reminder модели (не user message, как у watch).

**Транскрипт.** Строка claude-job: «claude <mode> · <model>» с живым прогрессом tool_use (существующие вложенные строки job), по завершении футер turns/duration/tokens/cost; usage-pane — строка claude (из ledger).

**Settings.** Раздел Claude только для чтения в v1: enabled, путь и версия бинаря, таблица режимов (model, effort, timeout) с подсказкой, что правится в ~/.cozyphi/claude.json.

**Тесты.** разбор @claude:mode; регистрация слэшей; mapper рендерит строку и футер; settings-раздел на фикстуре конфига.

**Зависит от:** claude-job-backend, claude-tool-and-gate, claude-economy-threads-brakes-ledger (треды и ledger).

## Acceptance Criteria

- @claude и @claude:mode превращаются в вызов тулы с verbatim task; /claude status|threads|verify работают и не создают user message.
- Строка claude-job показывает режим, модель, живой прогресс и футер с ценой; usage-pane показывает claude.
- Settings показывает статус и режимы; make fmt-check lint test rc=0.

## Verification Plan

1. go test ./internal/agent/... ./internal/tui/... -run 'Claude' -race.
2. Ручной прогон в TUI всех трёх слэшей и упоминания.
3. make fmt-check lint test.

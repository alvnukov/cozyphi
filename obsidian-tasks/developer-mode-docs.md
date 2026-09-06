---
id: developer-mode-docs
title: 17 — Документировать проверенный read-only developer mode
status: blocked
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
updated_at: "2026-09-06T09:10:07.752844Z"
---

## Body

**Что построить.** Пользовательское руководство по реализованному read-only developer mode и актуальные help/reference примеры. Читать epic developer-mode-readonly и завершённую coverage matrix.

**Blocked by:** developer-mode-coverage.

**Scope.** Документация и changelog, без изменения runtime и без перепроектирования interface. Сохранять язык конкретных существующих документов и английские CLI/служебные identifiers; task notes вести по-русски. Не расширять эту задачу до массовой очистки branding: remove-legacy-phi-branding отдельная задача. Если нужны изменения agent instructions, сначала применить writing-for-agents; в нормальном scope достаточно пользовательской документации.

**Работа.** Task worktree; проверки ссылок, примеров/help и diff, без Go gates всего repo. Commit/merge/ledger/cleanup по epic; push только по просьбе.

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

---
id: developer-mode-providers
title: 04 — Показать провайдеры и presence/source credentials без секретов
status: in_progress
priority: medium
model_level: medium
task_type: feature
parent_id: developer-mode-readonly
tags:
    - developer-mode
acceptance_criteria:
    - harness показывает состояние provider catalog и read-only импортов, источник выбранного provider/model и доступность данных.
    - Credentials представлены только presence/source; никаких значений, суффиксов, хешей и raw auth structures.
    - URL с userinfo/query и произвольные ошибки не раскрывают sentinel secrets ни в ответах, ни в audit/errors.
    - Чтение не обновляет каталог по сети, не читает лишние credential stores и не инициирует authentication.
    - Catalog/explain отличают импорт выключен, не загружен, ошибка загрузки и данные недоступны.
verification_plan:
    - 'Fixture catalogs/imports: enabled/disabled/load failure, источник выбора и пустой store.'
    - Sentinel secrets в ключах, URL userinfo/query, header-like fields и error text; проверка всех выходных каналов.
    - Spy network/auth adapters подтверждают нулевые обращения при catalog/snapshot/explain.
created_at: "2026-09-06T09:06:41.969375Z"
updated_at: "2026-09-06T11:07:41.99402Z"
---

## Body

**Что построить.** Расширить model-категорию готовыми безопасными сведениями о провайдерах, каталогах, импортах и наличии credentials. Читать epic developer-mode-readonly.

**Blocked by:** developer-mode-model.

**Scope.** Наблюдение существующих provider/import owners, не новая интеграция и не credential management. Добавлять allowlisted metadata рядом с владельцем; не передавать raw credentials/config в агрегатор. Безопасный source не должен включать credential value, секретный URL или произвольный фрагмент файла. Если owner не располагает фактом presence без side effect, вернуть unavailable, не выполнять auth probe. Все такие исключения явно перечислить.

**Работа.** Task worktree; профильные проверки без реальных credentials/сети; closeout по epic.

## Acceptance Criteria

- harness показывает состояние provider catalog и read-only импортов, источник выбранного provider/model и доступность данных.
- Credentials представлены только presence/source; никаких значений, суффиксов, хешей и raw auth structures.
- URL с userinfo/query и произвольные ошибки не раскрывают sentinel secrets ни в ответах, ни в audit/errors.
- Чтение не обновляет каталог по сети, не читает лишние credential stores и не инициирует authentication.
- Catalog/explain отличают импорт выключен, не загружен, ошибка загрузки и данные недоступны.

## Verification Plan

1. Fixture catalogs/imports: enabled/disabled/load failure, источник выбора и пустой store.
2. Sentinel secrets в ключах, URL userinfo/query, header-like fields и error text; проверка всех выходных каналов.
3. Spy network/auth adapters подтверждают нулевые обращения при catalog/snapshot/explain.

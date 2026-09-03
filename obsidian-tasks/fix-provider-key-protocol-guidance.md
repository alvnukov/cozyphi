---
id: fix-provider-key-protocol-guidance
title: предупреждение сниффинга советует provider, который протокол не выбирает
status: done
priority: medium
task_type: bug
parent_id: fix-provider-sniffing
tags:
    - config
    - llm
    - review-2026-09
acceptance_criteria:
    - Модель с явным provider из каталога не сниффится и не получает предупреждения, либо текст предупреждения не упоминает provider
    - /models показывает протокол из нормализованного конфига, не из повторного сниффинга
verification_plan:
    - go test ./internal/project/ ./cmd/
created_at: "2026-09-02T16:51:43.3229Z"
updated_at: "2026-09-02T19:39:40.274294Z"
---

## Body

**Проблема.** `internal/project/config.go:340` предупреждает «set protocol (or provider) explicitly», но `normalizeModelProtocol` смотрит только на имя модели и base_url; ключ `provider` (`ProviderID`, `config.go:411`) в выбор протокола не входит. Пользователь, добавивший `provider:`, остаётся со сниффингом и тем же предупреждением. Критерий задачи fix-provider-sniffing «явный provider в конфиге решает» не выполнен.

**Второе.** `cmd/config.go:242` (листинг `/models`) по-прежнему вызывает `llm.SniffProtocol` без учёта явного протокола и без предупреждения: одна функция, два вызова с разным контрактом.

**Как чинить.** Либо выводить протокол из известного provider (каталог провайдеров знает свой wire-протокол), либо убрать «(or provider)» из текста. Листинг `/models` брать протокол из нормализованного конфига. Найдено ревью правок после v0.19.0.

## Acceptance Criteria

- Модель с явным provider из каталога не сниффится и не получает предупреждения, либо текст предупреждения не упоминает provider
- /models показывает протокол из нормализованного конфига, не из повторного сниффинга

## Verification Plan

1. go test ./internal/project/ ./cmd/

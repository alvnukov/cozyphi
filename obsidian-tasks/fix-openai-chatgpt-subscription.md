---
id: fix-openai-chatgpt-subscription
title: Implement OpenAI ChatGPT subscription login correctly
status: done
priority: critical
task_type: bug
tags:
    - providers
    - openai
    - oauth
    - connect
    - security
acceptance_criteria:
    - /connect exposes OpenAI with ChatGPT Pro/Plus browser login as the primary method
    - Headless device authorization is an explicit fallback and API key remains a distinct OpenAI option
    - OAuth uses PKCE, random state, loopback callback validation, pinned Codex Responses endpoint, account id headers, and safe token refresh
    - No standalone misleading Codex provider remains in the catalog
    - Cancellation and failures are visible and do not block TUI input
verification_plan:
    - Add focused tests for browser PKCE URL, callback state/CSRF rejection, token exchange, cancellation, and OpenAI auth method selection
    - Run provider and TUI connect tests, then fmt-check lint test before commit
created_at: "2026-08-24T19:42:41.616132Z"
updated_at: "2026-09-02T22:17:19.62723Z"
---

## Body

Сделано в ветке bug/fix-openai-chatgpt-subscription, слито в main мержем 908c2d9 (коммит 1ba9d27).

**Что изменилось.** Провайдер `codex`, умевший только device code, убран. Вместо него один провайдер `openai` с тремя методами входа: браузерный Authorization Code + PKCE (основной путь к подписке ChatGPT Pro/Plus), device code как fallback для машины без браузера и API-ключ отдельным методом. Порядок задаёт `Info.Methods`, первый — основной.

**Где живут контракты.** Эндпоинт, протокол и список моделей переехали с провайдера на `AuthMethod`: обе подписочные ветки прибиты к `https://chatgpt.com/backend-api/codex` + `ProtocolOpenAIResponses`, ключ — к `https://api.openai.com/v1` + `ProtocolOpenAI`. Сохранённая учётка при загрузке сопоставляется обратно со своим методом (`credentialMethod`), поэтому ключ не может авторизовать запрос к подписочному эндпоинту, а `Connect` отказывает при попытке подменить базовый URL.

**Браузерный поток.** Слушатель loopback поднимается до сборки URL, redirect_uri выводится из реально полученного порта (в проде 1455, в тестах — свободный). state сравнивается через `subtle.ConstantTimeCompare`, подделанный колбэк получает 400 и до issuer не доходит. Отмена освобождает порт. После входа оба потока обновляют список моделей аккаунта, и он хранится в самой учётке, а не в общем каталоге.

**Миграция.** Старая oauth-учётка `codex` при старте переносится на `openai`, файл переписывается — те же модели возвращаются как `openai/…` без повторного входа. Ретиред-id удаляется при каждом merge каталога и не воскресает из кеша.

**UI.** В `/connect` появилась фаза выбора метода (↑↓/Tab, Enter, ← назад, Esc отмена) и скоуп подсказок `connect-method`. Сетевая работа идёт в фоне, ввод не блокируется, ошибки и предупреждение о неудавшемся обновлении моделей видны отдельно.

**Проверки.** Новые тесты: PKCE-челлендж и state в URL, отказ на несовпавшем state (400, учётка не создана), обмен кода с проверкой `SHA-256(verifier) == challenge` и закреплением контракта, отмена с освобождением порта, миграция codex→openai, подключение ключа на публичном эндпоинте, три метода в overlay. Тесты герметичны за счёт переписывающего RoundTripper: продовые URL остаются в коде, запросы идут на httptest. Гейт перед коммитом: `golangci-lint fmt --diff` чисто, `golangci-lint run` 0 issues, `go test ./...` exit 0 (90 пакетов ok).

Не пушилось: push делается только по явной просьбе.

## Acceptance Criteria

- /connect exposes OpenAI with ChatGPT Pro/Plus browser login as the primary method
- Headless device authorization is an explicit fallback and API key remains a distinct OpenAI option
- OAuth uses PKCE, random state, loopback callback validation, pinned Codex Responses endpoint, account id headers, and safe token refresh
- No standalone misleading Codex provider remains in the catalog
- Cancellation and failures are visible and do not block TUI input

## Verification Plan

1. Add focused tests for browser PKCE URL, callback state/CSRF rejection, token exchange, cancellation, and OpenAI auth method selection
2. Run provider and TUI connect tests, then fmt-check lint test before commit

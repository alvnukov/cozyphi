---
id: kimi-subscription-provider
title: 'Kimi по подписке: провайдер kimi-code (OAuth device flow)'
status: in_progress
priority: high
task_type: feature
branch: feature/kimi-subscription-provider
worktree_path: .worktrees/kimi-subscription-provider
verification_plan:
    - go build ./internal/provider/... ./internal/llm/...
    - 'go test ./internal/provider/... ./internal/llm/... (новые тесты: kimi-профиль device flow на httptest, refresh, /models декод, authenticator в openai-клиенте)'
    - один scoped golangci-lint run ./internal/provider/... ./internal/llm/...
    - живой вход по подписке при наличии аккаунта Kimi
    - CHANGELOG [Unreleased] + коммит на ветке задачи
created_at: "2026-09-15T08:24:35.332859Z"
updated_at: "2026-09-15T12:08:32.824612Z"
---

## Body

**What:** добавить в cozyphi провайдер Kimi (Moonshot) по подписке — переносом проверенной логики из плагина opencode https://github.com/t94j0/opencode-kimi-subscription (клон прочитан: /tmp/opencode-kimi-subscription).

**Facts confirmed by the plugin source (kimi CLI v0.26.0):**
- OAuth 2.0 Device Flow (RFC 8628) против https://auth.kimi.com: POST form /api/oauth/device_authorization {client_id: 17e5f671-d194-4dfb-9706-5516cb48c098} → {device_code, verification_uri(_complete), user_code, expires_in, interval}; поллинг POST form /api/oauth/token {client_id, device_code, grant_type=urn:ietf:params:oauth:grant-type:device_code}; ошибки authorization_pending/slow_down (+5с)/access_denied/expired_token; refresh: тот же /api/oauth/token {client_id, grant_type=refresh_token, refresh_token}.
- API: https://api.kimi.com/coding/v1 — OpenAI-совместимый, Bearer access (15 мин; плагин рефрешит при остатке <450с; на 401/403 — forced refresh + 1 ретрай).
- Модели: GET /models → {data:[{id, display_name, context_length, supports_reasoning, supports_image_in, supports_video_in, supports_tool_use, supports_thinking_type}]}; статический baseline: k3 (1048576), kimi-for-coding (262144), kimi-for-coding-highspeed (262144); output limit 65536.
- temperature строго = 1 (плагин переписывает тело, если задано иное).

**Implementation map (from cozyphi exploration):**
- internal/provider/manager.go builtinProviders(): новый провайдер kimi-code c AuthMethod oauth-device (BaseURL https://api.kimi.com/coding/v1, Protocol ProtocolOpenAI, Models = baseline).
- internal/provider/oauth.go — сегодня OpenAI-специфичен (issuer auth.openai.com, codexClientID, /api/accounts/deviceauth/*, заголовки ChatGPT-Account-Id/residency): нужен шов OAuth-профиля на провайдера (issuer/client_id/эндпоинты/кодеки/заголовки), два адаптера — openai (существующий) и kimi (новый).
- internal/llm/openai/client.go игнорирует cfg.Authenticator (только responses-клиент его применяет, responses/client.go:129) — научить openai-клиент Authenticator, иначе OAuth-провайдер на chat-completions не заработает.
- Обнаружение моделей: kimi_models.go по образцу codex_models.go, но OpenAI-форма {data:[...]}; валидация credential-моделей требует AccountID (storage.go:159) — у Kimi его нет: стабильный локальный ключ подключения.
- temperature: cozyphi шлёт только если задан в конфиге; пин = 1 при явном temperature ≠ 1 — решить (минимум: не задавать).

**Done:** провайдер собирается, /connect предлагает подписочный вход, модели видны, живой запрос при наличии токена. Гейты только по изменённым пакетам.

**Started (2026-09-15).** Взял задачу; план одобрен (R29), разведка провайдера и плагина завершена. Ветка feature/kimi-subscription-provider.

**Implemented (2026-09-15).** oauth.go: шов oauthGrant + grantFor, codex-логика в codexGrant; kimiGrant в kimi_oauth.go (RFC 8628: pending/slow_down +5с/denied/expired, refresh, Bearer + requestWithinBaseURL); kimi_models.go: discovery GET /models, TTL 6ч, тег кэша kimi-1, account-bound ключ kimiAccountID; manager.go: kimi-code в builtinProviders (baseline k3 1M / kimi-for-coding / highspeed, output 65536), RefreshSubscriptionModels = codex+kimi. internal/llm/openai: StreamChatCompletion и Compact принимают llm.ModelConfig и honoring RequestAuthenticator (паттерн responses-клиента). Решения: без 401/403 forced-refresh ретрая (единообразно с существующими), temperature не шлём если не задана, константный kimiAccountID. Тесты: device flow с httptest-сервером, denied, slow_down, refresh, подписка-only (APIKey → refusal), decode, authenticator-маршрутизация. Гейты зелёные: build/vet/test по изменённым пакетам, golangci-lint 0 issues; CHANGELOG [Unreleased].

**Follow-up (2026-09-15, вторая сессия).** Пользователь: «мне нужно как в kimi code». Проверка первоисточников: и в Python kimi-cli (src/kimi_cli/auth/oauth.py), и в новом TS-монорепо kimi-code (packages/oauth/src/oauth.ts, packages/agent-core-v2/src/human/credentials/kimi-oauth.ts) вход — тот же RFC 8628 device grant (client_id 17e5f671-…, /api/oauth/device_authorization + /api/oauth/token); «браузерность» Kimi Code = CLI сам открывает webbrowser.open(verification_uri_complete) (oauth.py:648), код вшит в URL. Скрин пользователя («kimi-for-coding, Sign-in: API key») — это каталожная запись models.dev (@ai-sdk/anthropic), которая приезжает в /connect независимо от ветки; подписка — отдельный провайдер kimi-code. Сделано: device flow в view.go теперь открывает verification URL в браузере (util.OpenBrowser), при неудаче URL+код остаются на экране; ProviderDeviceCodeMsg получил BrowserErrText, оверлей рендерит «Browser did not open automatically» (по образцу browser-flow). Попутно починен пропущенный утром тест controller (TestTheHarnessNamesTheProviderCatalogItActuallyHolds: 2→3 builtin, счёт каталога 3→4). Каталожный двойник kimi-for-coding в /connect остался — возможен follow-up про скрытие/переопределение.

**Note (2026-09-15).** **Review fixes (2026-09-15, третья сессия).** По итогам двухосного ревью ветки: (1) Temperature: дефолт = 1 на модельных записях kimi-code в manager.Models() (manager.go) — endpoint принимает только 1 (факт из плагина opencode-kimi-subscription); НЕ пин в клиенте — модель из проектного конфига/opencode-импорта с тем же именем и своими Options переопределяет через обычный merge. Тест TestManagerDefaultsKimiTemperatureToOne. Решение по явному temperature ≠ 1 из спеки таким образом закрыто: по умолчанию 1, изменение — через конфиг. (2) Метода OpenAI device flow переименована «ChatGPT Pro/Plus (headless device code)» → «(device code)» — после автооткрытия браузера «headless» врёт. (3) Решение про refresh skew зафиксировано: cozyphi рефрешит токен за 30с до истечения против плагиновских 450с; в связке с решением «без forced-refresh ретрая на 401/403» длинный стрим (>15 мин от выдачи токена) может умереть в середине — осознанное отклонение от фактов плагина, единообразно с поведением других провайдеров cozyphi; при живой боли — отдельная доработка. CHANGELOG [Unreleased] дополнен, коммит на ветке.

## Verification Plan

1. go build ./internal/provider/... ./internal/llm/...
2. go test ./internal/provider/... ./internal/llm/... (новые тесты: kimi-профиль device flow на httptest, refresh, /models декод, authenticator в openai-клиенте)
3. один scoped golangci-lint run ./internal/provider/... ./internal/llm/...
4. живой вход по подписке при наличии аккаунта Kimi
5. CHANGELOG [Unreleased] + коммит на ветке задачи

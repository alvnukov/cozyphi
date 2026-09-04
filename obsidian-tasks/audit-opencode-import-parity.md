---
id: audit-opencode-import-parity
title: Port opencode config logic from ~/src/opencode into the import
status: done
priority: high
task_type: chore
tags:
    - opencode
    - audit
branch: chore/audit-opencode-import-parity
worktree_path: .worktrees/audit-opencode-import-parity
acceptance_criteria:
    - Каждое утверждение F1–F10 подтверждено кодом ~/src/opencode с file:line (основная сессия сверяет цитаты чтением)
    - 'Составлен porting-спек: какие места cozyphi internal/opencode расходятся с opencode и что меняется'
    - Логика конфига (загрузка, раскрытие ссылок, разрешение провайдеров/моделей) перенесена из opencode; расхождения — только осознанные, с обоснованием в доке
    - Тесты покрывают каждое изменение; существующие тесты обновлены только там, где семантика меняется сознательно
    - Scoped-гейты зелёные; CHANGELOG [Unreleased] обновлён
    - Закоммичено и смержено в main (--no-ff), задача закрыта через task done
verification_plan:
    - Each F1–F10 claim answered from opencode source with file:line, cross-checked by main session reading the cited lines
    - Discrepancy table reviewed against cozyphi internal/opencode implementation
    - No cozyphi code changes without a confirmed divergence and a follow-up task
created_at: "2026-09-04T13:37:29.93812Z"
updated_at: "2026-09-04T14:48:31.040017Z"
---

## Body

**Goal** Take cozyphi's opencode-import config logic from the actual opencode source at ~/src/opencode (user directive 2026-09-04: «нужно взять логику конфига из опенкода»), replacing assumptions landed in 642d916/479b134 (provider import) and 0b1bfaa/e6b13ed ({file:} in MCP headers).

**Claims to verify with file:line citations from ~/src/opencode, then port**
- F1 raw-text {env:NAME} pass over the whole config before JSON parse — exists? env-only?
- F2 {file:PATH} expanded in the same raw pass (anywhere in config) or post-parse on typed fields?
- F3 if raw pass: trimming, missing-file behavior, JSON-escaping of content?
- F4 which fields accept references ({env:}/{file:}/{prompt:}) — apiKey, mcp headers, mcp environment, url, baseURL?
- F5 precedence: auth.json key vs options.apiKey?
- F6 keyless config-declared providers instantiated when baseURL+models present?
- F7 config provider.models vs models.dev catalog: overlay per id, replace, limit semantics?
- F8 disabled_providers: exact top-level shape and effect?
- F9 {env:} inside {file:} content or nesting — recursive resolution?
- F10 provider/model sort order in the picker.

**Deliverable** Ported semantics: cozyphi's internal/opencode mirrors opencode's config pipeline (loader, reference expansion, provider/model resolution, auth); every intentional deviation documented with rationale. Discrepancy table in the task note.

**Note (2026-09-04).** 2026-09-04 audit vs opencode@4161695 (2026-08-24). Verified citations: config/variable.ts:34-91 (substitute: env pass whole text, then single file pass; // line rule; ~/ and configDir-relative; trim + JSON.stringify escape; missing file → InvalidError, default missing:"error"); config/config.ts:246-260 (global = config.json → opencode.json → opencode.jsonc, deep-merged; failure → {}), 281-289; provider/provider.ts:1457-1466 (config providers always enter database), 1468-1552 (models overlay per id, ?? nullish; limit.context ?? chain at 1533-1537; api url chain at 1490: model.provider.api ?? provider.api ?? existing.api.url ?? modelsDev ?? ""), 1557-1581 (credential layers: env vars then auth.json overwrites), 1622-1630 + 1756 (config re-apply; options.apiKey wins), 1690-1693 (zero-model providers deleted), 1646-1651 (isProviderAllowed delete).

Discrepancies found (cozyphi → opencode): D1 {file:} must be a raw-text pass (not post-parse MCP-only), escaped+trimmed, missing → hard error; D2 no empty-on-missing; D3 three global config files deep-merged in order; D4 credential ladder flipped (config apiKey > auth.json > catalog env vars); D5 apiKey object form {"env":..}/{"file":..} does not exist — remove; D6 models: map key is the model id, model.id is the API id; limit ?? nullish (0 wins), present-value overrides; provider.api string is a URL source alongside options.baseURL; D7 enabled_providers allowlist missing (disabled wins); D8 config provider without baseURL is still instantiated (npm defaults openai-compatible); D9 catalog env-var credential layer missing; D10/D11 mcp shape and sorting match or are documented deviations.

Porting spec slices: S1 substitution engine (D1+D2, delete post-parse expander + object forms); S2 file set + deep merge (D3, incl. OPENCODE_CONFIG_DIR); S3 credentials + models overlay (D4+D6+D9, drop baseURL skip); S4 enabled_providers (D7); S5 docs/deviations + CHANGELOG.

**Done (2026-09-04).** 2026-09-04: Landed. Branch chore/audit-opencode-import-parity @ f99020e, merged --no-ff into main as 2319760; worktree and branch removed. Scope: full port of opencode config semantics into internal/opencode per the D1–D11 table — {env:}/{file:} substitution over raw text (trimmed, JSON-escaped, missing file = hard error), three global config files deep-merged in order, credential ladder config apiKey > auth.json, models overlay (map key = id, model.id = API id, nullish limits with 0 winning), enabled/disabled_providers. Spec review (S1–S5 PASS) surfaced 4 parity bugs vs real opencode, all verified against ~/src/opencode quotes and fixed: (1) options.baseURL beats provider.api (provider.ts:1734-1736); (2) provider.api moves only config-listed models (provider.ts:1468-1490); (3) OPENCODE_CONFIG merges over the globals (config.ts:398-404); (4) explicit apiKey:"" suppresses auth fallback (provider.ts:1756) and enabled_providers:[] allows nothing. Documented deviations (D9 catalog env layer, per-model provider overrides, costs/variants, limit.input) in doc/opencode.md. Gates: build/test/lint green in worktree and on main after merge. Pending: user verifies /model shows opencode/beeline/... and mcp-gateway sends no 401.

## Acceptance Criteria

- Каждое утверждение F1–F10 подтверждено кодом ~/src/opencode с file:line (основная сессия сверяет цитаты чтением)
- Составлен porting-спек: какие места cozyphi internal/opencode расходятся с opencode и что меняется
- Логика конфига (загрузка, раскрытие ссылок, разрешение провайдеров/моделей) перенесена из opencode; расхождения — только осознанные, с обоснованием в доке
- Тесты покрывают каждое изменение; существующие тесты обновлены только там, где семантика меняется сознательно
- Scoped-гейты зелёные; CHANGELOG [Unreleased] обновлён
- Закоммичено и смержено в main (--no-ff), задача закрыта через task done

## Verification Plan

1. Each F1–F10 claim answered from opencode source with file:line, cross-checked by main session reading the cited lines
2. Discrepancy table reviewed against cozyphi internal/opencode implementation
3. No cozyphi code changes without a confirmed divergence and a follow-up task

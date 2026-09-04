---
id: audit-opencode-import-parity
title: Port opencode config logic from ~/src/opencode into the import
status: in_progress
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
updated_at: "2026-09-04T13:52:22.84836Z"
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

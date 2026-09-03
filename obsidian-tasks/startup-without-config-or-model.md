---
id: startup-without-config-or-model
title: Start without a config file or a default model
status: done
priority: high
model_level: very_high
task_type: feature
tags:
    - startup
    - config
    - ux
acceptance_criteria:
    - Starting with no ~/.cozyphi/config.yaml creates it with the default template (mode 0600) and the TUI starts; an existing file is never overwritten or rewritten.
    - Parsing the default template yields a Config equal to the built-in defaults used for a missing file (unit test).
    - A config with no models loads without error; Model() returns a zero model and does not add an entry to Models.
    - A configured model without api_key loads with a warning instead of an error; an entry without a name still errors.
    - COZYPHI_MODEL/COZYPHI_API_KEY/COZYPHI_BASE_URL still create or override the default entry as before.
    - NewController succeeds with an empty config and no providers; with an empty config and a non-empty catalog it selects the first catalog model.
    - With no model selected, submitting a prompt does not start a turn or open a connection and the user sees a hint naming /connect and /model.
    - Footer shows a placeholder instead of an empty model name.
    - cozyphi run without a resolvable model exits with a clear error.
    - README, docs and CHANGELOG updated; make fmt-check lint test passes.
verification_plan:
    - go test ./internal/project/... ./internal/tui/... ./cmd/... with new tests for template invariant, config creation, zero-model loading, controller fallback and submit guard.
    - make fmt-check lint test in the worktree.
    - 'Manual: HOME with no .cozyphi dir, run the binary, confirm config.yaml appears and the TUI starts with the no-model notice; then with opencode models present confirm a model is auto-selected.'
created_at: "2026-09-03T08:30:59.729743Z"
updated_at: "2026-09-03T11:07:32.235846Z"
---

## Body

**Problem** A fresh install cannot start: `loadConfig` in `internal/project/config.go` fails on zero models, a missing default model name or a missing api_key, and `cozyphi` exits with "Configure a model first, then restart". The user must hand-write `~/.cozyphi/config.yaml` before ever seeing the TUI, although `/connect`, the provider store and the OpenCode source already supply models at runtime.

**Change** `Project.LoadConfig()` silently creates `~/.cozyphi/config.yaml` from a commented default template (owner-only 0600) when the file is missing and never overwrites an existing one; a write failure becomes a config warning, not a fatal error. Parsing the template must produce exactly the built-in defaults (tested). Zero models and a missing api_key stop being errors (a configured entry without a name still is; a missing key produces a warning). `Config.Model()` returns a zero model without mutating the config when no model is configured; only COZYPHI_MODEL/COZYPHI_API_KEY/COZYPHI_BASE_URL create an entry.

**Startup model** Session model is resolved in this order: COZYPHI_MODEL override, config default, last used model from ui.json, first model from the runtime catalog (connected providers, then OpenCode), none. The TUI always starts. With no model at all the footer shows a placeholder, a startup notice explains how to get one (/connect, /model, config.yaml), and submitting a prompt is refused with the same hint without any HTTP request. When a fallback model was picked automatically the notice names it. Headless `cozyphi run` without a resolvable model fails with a clear error naming the three ways to configure one.

**Docs** README quick start, doc pages that describe first-run configuration, CHANGELOG `## [Unreleased]`.

**Out of scope** interactive onboarding wizard, changes to `cozyphi config` web editor beyond allowing an empty api_key, provider store changes.

**Done (2026-09-03).** Залендилось: merge 685689e (feature commit e28543c, ветка feature/startup-without-config-or-model). Первый старт сажает закомментированный ~/.cozyphi/config.yaml (O_EXCL 0600, существующий не трогается, ошибка записи = warning, парсинг шаблона == встроенным дефолтам — тестом); ноль моделей и пустой api_key грузятся (без name — ошибка, без ключа — warning), Model() не мутирует конфиг; резолв модели COZYPHI_MODEL → дефолт конфига → LastModel из ui.json → первая модель каталога (провайдеры → opencode) → none; TUI стартует всегда: плейсхолдер "no model", startup-нотис (называет автоподбор), отказ сабмита/woke/resume без хода и HTTP; resume сохраняет модель сессии; headless cozyphi run резолвит той же цепочкой и выходит с ошибкой, называющей три способа; веб-редактор cozyphi config принимает пустой api_key. Гейт make fmt-check lint test зелёный в worktree, sanity go test на main зелёный. README/doc/tui.md/CHANGELOG обновлены. Реализация: два субагента + фикс-воркер по итогам двухосевого ревью (web-editor api_key, headless-цепочка, guard в startPromptLocked, resume-edge, дедупликации).

## Acceptance Criteria

- Starting with no ~/.cozyphi/config.yaml creates it with the default template (mode 0600) and the TUI starts; an existing file is never overwritten or rewritten.
- Parsing the default template yields a Config equal to the built-in defaults used for a missing file (unit test).
- A config with no models loads without error; Model() returns a zero model and does not add an entry to Models.
- A configured model without api_key loads with a warning instead of an error; an entry without a name still errors.
- COZYPHI_MODEL/COZYPHI_API_KEY/COZYPHI_BASE_URL still create or override the default entry as before.
- NewController succeeds with an empty config and no providers; with an empty config and a non-empty catalog it selects the first catalog model.
- With no model selected, submitting a prompt does not start a turn or open a connection and the user sees a hint naming /connect and /model.
- Footer shows a placeholder instead of an empty model name.
- cozyphi run without a resolvable model exits with a clear error.
- README, docs and CHANGELOG updated; make fmt-check lint test passes.

## Verification Plan

1. go test ./internal/project/... ./internal/tui/... ./cmd/... with new tests for template invariant, config creation, zero-model loading, controller fallback and submit guard.
2. make fmt-check lint test in the worktree.
3. Manual: HOME with no .cozyphi dir, run the binary, confirm config.yaml appears and the TUI starts with the no-model notice; then with opencode models present confirm a model is auto-selected.

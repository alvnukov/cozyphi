---
id: codex-reasoning-effort
title: Switch reasoning effort for Codex models
status: done
priority: high
task_type: feature
tags:
    - provider
    - codex
    - tui
created_at: "2026-08-24T22:00:00.000000Z"
updated_at: "2026-08-24T22:00:00.000000Z"
---

## Body

Expose OpenAI Codex reasoning effort as switchable model variants. Codex provider
models should keep their wire model id while selecting a reasoning effort level
(minimal/low/medium/high) so existing `/model` and settings model selection can
choose how much reasoning the model spends.

source_repo_path: /Users/zol/src/cozyphi

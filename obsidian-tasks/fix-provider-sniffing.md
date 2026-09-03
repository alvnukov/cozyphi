---
id: fix-provider-sniffing
title: провайдер определяется нюхом имени, в двух местах
status: todo
priority: medium
task_type: bug
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - явный provider в конфиге решает
    - сниффинг один, в одном месте, с предупреждением
verification_plan:
    - go test ./internal/llm/client/... ./cmd/...
created_at: "2026-08-23T15:17:22.112734Z"
updated_at: "2026-08-23T15:17:22.112734Z"
---

## Body

internal/llm/client/client.go:69-75 (Contains anthropic / HasPrefix claude) и дубль в cmd/config.go:304-307. Любой OpenAI-совместимый шлюз (LiteLLM, OpenRouter, корпоративный прокси) с именем claude-* молча переключается на wire-формат Anthropic. Фикс: явное поле provider в ModelConfig, сниффинг только fallback-ом с предупреждением.

## Acceptance Criteria

- явный provider в конфиге решает
- сниффинг один, в одном месте, с предупреждением

## Verification Plan

1. go test ./internal/llm/client/... ./cmd/...

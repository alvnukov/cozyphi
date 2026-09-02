---
id: ci-red-main-gates
title: 'CI-red main: находки golangci-lint v2.13.0 и TZ-зависимые тесты session'
status: done
priority: high
tags:
    - ci
    - lint
    - tests
    - tz
created_at: "2026-08-30T20:52:51.839657Z"
updated_at: "2026-08-30T20:52:51.839657Z"
---

## Body

Ран 33329579511 на 67fcf31 красный по двум причинам. (1) Дрейф 2.12.2→v2.13.0: 4× errors.AsType (internal/lsp/query.go, internal/proc/proc.go, internal/tui/commands/registry.go, internal/tui/editor/editor.go) + gofumpt в internal/tui/ctxpane/pane_view_test.go. (2) 3 TZ-зависимых теста internal/session: JSON round-trip 'Z'-таймстемпов даёт loc=nil, а Now().Round(0) — Local; roundPlanTimes канонизирует ожидание через тот же JSON round-trip. Всё исправлено, полный гейт зелёный (lint/fmt/test и race — exit 0).

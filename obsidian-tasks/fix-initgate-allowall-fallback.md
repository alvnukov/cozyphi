---
id: fix-initgate-allowall-fallback
title: 'initGate ставит AllowAll при ошибке сборки gate: fail-open путь остался'
status: done
priority: high
task_type: bug
parent_id: fail-closed-permission-assembly
tags:
    - security
    - permissions
    - fail-closed
    - review-2026-09
acceptance_criteria:
    - Двойная ошибка NewGate даёт gate, отвечающий Deny с actionable-причиной, а не AllowAll
    - Пользователь видит, что gate не собрался
    - Тест на реконфигурацию гоняет запросы параллельно с re-init и не находит окна Allow
verification_plan:
    - go test -race ./internal/tui/controller/ ./internal/permission/
created_at: "2026-09-02T16:51:43.322481Z"
updated_at: "2026-09-02T19:26:04.067301Z"
---

## Body

**Проблема.** `internal/tui/controller/controller.go:417-422`: если `permission.NewGate` падает и для настроенной, и для дефолтной политики, контроллер ставит `BypassGate{Inner: permission.AllowAll{}}`. С выключенным bypass BypassGate делегирует в Inner, то есть разрешает всё. После ветки permission-symlink-workspace-escape `NewGate` стал fallible (резолв путей), так что путь достижим: пропавший cwd под процессом даёт пустой `WorkspaceRoot()`, `ResolveTarget` с пустой строкой падает дважды, re-init через SetModel оставляет все запросы разрешёнными. Противоречит критерию задачи fail-closed-permission-assembly «Only an explicit enabled bypass can return unconditional Allow». `TestInitGateReconfigurationStaysCompleteAndClosed` эту ветку не задевает.

**Дополнительно.** План верификации той задачи просил гонку реконфигурации с входящими запросами; сделано пять последовательных re-init без запросов, headless-сборка (`phi run`) не покрыта.

**Как чинить.** Fallback на gate, отвечающий Deny с причиной «permission gate failed to assemble: …», и тост или лог пользователю; тест на двойную ошибку NewGate; параллельный тест reconfigure под нагрузкой запросов. Найдено ревью правок после v0.19.0.

## Acceptance Criteria

- Двойная ошибка NewGate даёт gate, отвечающий Deny с actionable-причиной, а не AllowAll
- Пользователь видит, что gate не собрался
- Тест на реконфигурацию гоняет запросы параллельно с re-init и не находит окна Allow

## Verification Plan

1. go test -race ./internal/tui/controller/ ./internal/permission/

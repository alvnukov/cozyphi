---
id: refactor-engine-reconfigure
title: 'Engine: атомарный reconfigure вместо caller-remembered dance из 6 шагов'
status: done
priority: high
task_type: refactor
parent_id: cozyphi-enterprise-code-review
branch: refactor/refactor-engine-reconfigure
worktree_path: .worktrees/refactor-engine-reconfigure
acceptance_criteria:
    - порядок инициализации знает только Engine
    - readonly-движок переживает reconfigure
    - одна пересборка на смену модели
verification_plan:
    - go test ./internal/agent/... ./internal/tui/controller/...
created_at: "2026-08-23T15:17:22.116821Z"
updated_at: "2026-09-05T22:00:11.070209Z"
---

## Body

Движок — mutable god object: клиент, executor, гейт, ask, jobs, hooks, mcp, компакция, пересборка через 7 сеттеров. Контроллер обязан звать Cancel -> initGate -> SetPermission -> SetContinueAsk -> SetJobs -> ReloadHooks -> SetModel по порядку (controller.go:352-387); SetJobs и SetModel каждый пересобирает executor+client+prompt (двойная пересборка на смену модели); SetPermission тычет engine.executor.gate напрямую (:209-213). Кандидат: хранить базовый список тулов и один reconfigure(model, gate, ask, jobs, hooks), пересобирающий всё атомарно. Поглощает fix-rebind-tools-resets-readonly и знание порядка из контроллера. Deletion test: удаление любого из пяти методов пересборки только двигает баг — граница module не там.

**Started (2026-09-06).** Taking the approved atomic session-local model/effort slice on current main, including in-flight snapshot, rollback, read-only role and plan override preservation. Parent inbox already landed; no lint rerun or broad core review.

**Note (2026-09-06).** User forbids agents; both interrupted workers were cancelled and all further changes/checks are direct. Inspected retained model patch: one Engine selection commit, immutable inference/tool runtime, validation rollback and parent/sibling/read-only tests. w29 agent/controller tests passed. Directly wired pending/effective/plan-selected composer state and corrected configured-default selection projection; formatting/UI checks w32 running. Source/tool guidance no longer mandates wait in interactive mode. No lint rerun or push.

**Done (2026-09-06).** Landed main75ade1c via verified integration e69fe05 (model01f884f, selector6c0ddbe). Engine.SelectModel validates before one commit; active inference and tool round retain runtime snapshot; next round receives session-local selection. Controller no longer performs gate/hooks/jobs reload dance; read-only and parent/sibling isolation and plan override covered. Pending/effective/selected state shown in composer. W33 formatter/diffcheck, scoped agent model races, controller/sessions/editor/notify/chat/prompt races and ./cmd build passed. Synthetic live terminal smoke verified held A→selected B pending label→next request B, parent unchanged A; clickable selector and background attention preserved focus/unread semantics. Evidence /tmp/cozyphi-final-selector-{pending,parent,child}.txt and requests.jsonl. No lint rerun or push. Pre-existing source-only context-ceiling concern tracked separately in child-model-context-ceiling.

## Acceptance Criteria

- порядок инициализации знает только Engine
- readonly-движок переживает reconfigure
- одна пересборка на смену модели

## Verification Plan

1. go test ./internal/agent/... ./internal/tui/controller/...

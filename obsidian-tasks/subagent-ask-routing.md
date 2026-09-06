---
id: subagent-ask-routing
title: D — Ask ребёнка всплывает в родителе с префиксом role(описание)
status: todo
priority: high
model_level: high
task_type: feature
parent_id: subagent-panel-ux
tags:
    - agents
    - tui
    - permissions
branch: feature/subagent-ask-routing
worktree_path: .worktrees/subagent-ask-routing
acceptance_criteria:
    - Пока View ребёнка не текущий экран, его PermissionAskMsg/ContinueAskMsg/QuestionAskMsg показываются в overlay текущего экрана семейства (родителя или сиблинга) с префиксом [role(описание)]; когда ребёнок текущий — как сейчас, у него.
    - Approve пропускает вызов; Esc отклоняет только этот вызов (ребёнок не останавливается); «Allow All for This Session» действует только на сессию ребёнка в пределах role-ceiling; «Allow All for Every Session» из ask ребёнка недоступен или явно предупреждает; в родителя гранты не текут.
    - Строка панели ребёнка показывает ⏸ … waiting: <kind>; внимание поднимается у родительской вкладки (notice line), а не уводит в ребёнка; завершение ребёнка не даёт OS-уведомления (turn end/wake родителя — дают).
    - Ответ приходит в тот же Reply-канал ребёнка; переключение экрана во время открытого ask не теряет и не дублирует ask (dismiss закрывает overlay, где бы он ни был показан).
verification_plan:
    - Тесты роутинга: ask ребёнка при неактивном View → overlay родителя с префиксом; approve/Esc → ответ в канал ребёнка; Esc не останавливает assignment; AllowSession в ask ребёнка не меняет allowAll родителя.
    - Тест смены экрана при открытом ask (перенос/закрытие без дублей).
    - Гейты по изменённым пакетам: gofumpt/golines, go build ./cmd, go test -race по sessions/overlays/controller/editor, один golangci-lint run.
created_at: "2026-09-06T19:35:00Z"
updated_at: "2026-09-06T19:35:00Z"
---

## Body

Фаза D эпика subagent-panel-ux — пункт 7 «Target contract» в `AGENTS_DESIGN.md`. Стартует после слияния B2 (семейство и текущий экран).

**Стартовые точки.** `internal/tui/controller/controller.go` `askPermission`/`askContinue`/`askQuestion` (+ generic `ask` с Reply/dismiss; `AllowSession` ставит `c.allowAll` — у ребёнка это ceiling роли через `initGate`); `internal/tui/sessions/view.go` `drainBus` case PermissionAskMsg/ContinueAskMsg/QuestionAskMsg (667–680) → `overlays`; `internal/tui/overlays/overlays.go` (`beginPermissionAsk`, `describeAsk` header — место для префикса), `lifecycle.go` `recordStatus` (Waiting/attention), `attention.go`; `internal/tui/editor/editor.go` notice line. Шину ребёнка дренирует его View (скрытый) — маршрут: View ребёнка при неактивности отдаёт ask семейству, семейство показывает в текущем View.

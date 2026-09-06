---
id: subagent-ask-follows-screen
title: Ask семьи следует за текущим экраном, а не приклеен к месту показа
status: done
priority: high
model_level: high
task_type: feature
parent_id: subagent-panel-ux
tags:
    - agents
    - tui
    - permissions
branch: feature/subagent-ask-follows-screen
worktree_path: .worktrees/subagent-ask-follows-screen
acceptance_criteria:
    - Семья держит один слот ожидающего ask (permission/continue/question) с origin; рисует его тот экран семьи, который сейчас текущий (родитель или любой ребёнок); при смене экрана ask переезжает вместе с пользователем, без дублей и без потери; ответ с любого экрана уходит в исходный Reply-канал, слот пустеет.
    - Симметрично для родителя — ask родителя, поднятый пока текущий экран — ребёнок, показывается на экране ребёнка с меткой [main]; ask ребёнка на экране родителя или сиблинга — с меткой [role(описание)]; на собственном экране источника метки нет.
    - Если на текущем экране уже открыт ask, следующий ждёт в очереди слота (строка панели ⏸ waiting как сейчас) и показывается после ответа; освобождение ребёнка с ask в очереди = deny; закрытие родителя = deny всем.
    - Поведение «Allow All for Every Session» скрыт у ask ребёнка, scope к сессии-источнику, внимание у вкладки родителя — не меняется.
verification_plan:
    - Тесты sessions: ask ребёнка при экране родителя → overlay родителя; ShowChild → overlay ребёнка без метки, у родителя пусто; ShowMain → обратно; ответ на любом экране → канал ребёнка, слот пуст; ask родителя при экране ребёнка → у ребёнка с [main]; очередь двух ask; release/close с ask в очереди → deny.
    - Гейты по изменённым пакетам: golangci-lint fmt по файлам, go build ./cmd, go test -race по sessions/overlays/editor, один golangci-lint run.
created_at: "2026-09-06T22:50:00Z"
updated_at: "2026-09-06T23:20:00Z"
---

## Body

Решение пользователя 2026-09-06 после фазы D: ask должен быть виден «и там и там» — реализуем как один ask, следующий за текущим экраном семьи, а не как две копии. Стартовые точки: `internal/tui/sessions/ask.go` (askHost/showAsk/withdrawAsk), `family.go` (Screen/Show/hosts/Release), `overlays` (`AskOrigin`, `ApplyFrom`, `DenyFrom`), `editor.go` ShowChild/ShowMain, `lifecycle.go` recordStatus.

**Done (2026-09-06).** Слито в main (b1a0cce): `sessions/family_ask.go` — очередь pendingAsk у семьи, голову рисует `Family.Screen()`, `showHead/withdrawAsk/askResolved/denyAll`; `overlays.Withdraw(owner)` вместо `DenyFrom`, `SetAskResolved` seam, `AskOrigin.Child`; метки `[role(описание)]` и `[main]` (имя из реестра); `Editor.ShowChild` вызывает `Family.Show`; `Status.Waiting` снимает семья; `BeginClose` родителя отклоняет всю очередь. Доки, CHANGELOG (пункт фазы D поправлен на месте), AGENTS_DESIGN.md. Гейты по sessions/overlays/editor зелёные.

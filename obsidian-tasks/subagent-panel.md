---
id: subagent-panel
title: B2 — Проводка панели сабагентов, семейство детей вне селектора, ↓/↑ между композером и панелью
status: in_progress
priority: high
model_level: high
task_type: feature
parent_id: subagent-panel-ux
tags:
    - agents
    - tui
    - multisession
branch: feature/subagent-panel
worktree_path: .worktrees/subagent-panel
acceptance_criteria:
    - Дети не попадают в Registry/селектор; родительский View держит их в своём семействе с cap 12 (первым освобождается самый старый завершённый; при 12 работающих — отказ как сейчас); job.Manager.MaxConcurrent ограничивает работающих; проверка в Runtime.newChild считает только детей.
    - Панель (agentpanel) рисуется между композером и футером у родителя и у каждого его ребёнка, одна на семейство; строка текущего экрана помечена ●; строки собираются из снимка (статус/ожидание из View.Status и Assignment, имя role(описание), счётчик tools и старт из SubagentStore фазы A).
    - Enter на строке ребёнка показывает его View текущим экраном без вкладки; Enter на main возвращает родителя; ↓ из композера при курсоре на последней визуальной строке (или пустом композере) переводит фокус в панель; ↑ на main и Esc возвращают в композер; Esc внутри композера ребёнка остаётся interrupt.
    - x останавливает работающего ребёнка тем же путём, что agent_cancel; успешный ребёнок уходит из панели сразу, футер 30 с показывает подсказку /agents; ошибка/стоп держатся 30 с.
    - Editor учитывает детей: DrainNow дренирует их шины, Close закрывает, AcceptInterrupt отказывает в выходе при работающих детях, /close на экране ребёнка лишь возвращает родителя; уведомление об attention (#N … /switch N) детей не касается.
    - doc/tui.md «Interactive child assignments» и AGENTS_DESIGN.md п.3–5, 8 приведены к реализации; CHANGELOG Unreleased.
verification_plan:
    - Тесты editor: ребёнок не в Entries/селекторе; ShowChild/ShowMain; AcceptInterrupt при работающем ребёнке; /close на экране ребёнка.
    - Тесты sessions: семейство cap 12 и порядок освобождения; ↓ из композера в панель и возврат; строки панели из Status/Assignment/store.
    - Тесты controller: отмена ребёнка из UI тем же путём, что agent_cancel.
    - tmux smoke: два ребёнка, панель, вход в ребёнка и возврат, x, подсказка футера.
    - Гейты по изменённым пакетам: gofumpt/golines, go build ./cmd, go test -race по editor/sessions/controller/agentpanel/cmd, один golangci-lint run.
created_at: "2026-09-06T19:20:00Z"
updated_at: "2026-09-06T20:05:00Z"
---

## Body

Вторая половина фазы B эпика subagent-panel-ux (пункты 3, 4, 5, 8 «Target contract» в `AGENTS_DESIGN.md`). Стартует после слияния subagent-row-progress (A) и subagent-panel-widget (B1).

**Стартовые точки.** `cmd/main.go` + `cmd/session_ui.go` (`newChildSessionSync` кладёт детей в Registry — заменить на семейство родителя); `internal/tui/editor/editor.go` (`syncSelection`, `drawShell`, `AcceptInterrupt`, `Close`, `DrainNow`), `session_close.go` (`CloseCurrent`); `internal/tui/sessions/view.go` (`Draw` — раскладка chat/footer через `slot.Arbitrate`, `Handle` — лестница событий, `Focus`), `lifecycle.go` (`Status`, `recordStatus`), `attention.go`; `internal/components/chat/chat_input.go` (KeyDown при `atEnd` → seam «уйти вниз»); `internal/tui/controller/children.go` (`ChildSession` без ссылки на родителя — добавить ParentSessionID; cap в `newChild`); `internal/job/manager.go` `Cancel`; `internal/tui/footer/footer.go` (временная подсказка рядом с `updateHint`).

**Started (2026-09-06).** A и B1 слиты; работа в `.worktrees/subagent-panel` на `feature/subagent-panel`.

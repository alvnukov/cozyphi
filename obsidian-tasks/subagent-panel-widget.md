---
id: subagent-panel-widget
title: B1 — Виджет панели сабагентов (internal/tui/agentpanel) на browse-ките
status: in_progress
priority: high
model_level: high
task_type: feature
parent_id: subagent-panel-ux
tags:
    - agents
    - tui
    - components
branch: feature/subagent-panel-widget
worktree_path: .worktrees/subagent-panel-widget
acceptance_criteria:
    - Новый пакет internal/tui/agentpanel — тупой виджет над снимком строк (rows func() []Row), без доступа к job/controller; часы инжектируются (now func() time.Time).
    - Первая строка main, дальше по строке на ребёнка; ● у текущего экрана, ○ у остальных; глифы ⟳ running, ⏸ waiting: <kind>, ✗ failed, ■ stopped; суффикс · N tools · elapsed; elapsed тикает через WakeIn, после терминала замораживается.
    - Окно до трёх строк; лишние — прокруткой по диалекту DESIGN.md (↑↓/j k, gg/G, Ctrl+U/D, PgUp/PgDn, колесо); неселектируемые индикаторы «↑ N more» / «↓ N more», когда строки скрыты сверху/снизу.
    - Done-строка исчезает сразу и на 30 с взводит подсказку футера (Hint()); failed/stopped держатся 30 с от Ended, x убирает раньше; x на running вызывает onStop; таймеры через WakeAt, без горутин.
    - Enter/Space — onOpen(id) (пустой id = main); Esc или ↑ на строке main — onLeave; клик выбирает и открывает; колесо крутит окно.
    - Подсказки только из каталога internal/tui/keys (новый scope), строки закреплены тестом keys_test; DESIGN.md — таблица adoption.
verification_plan:
    - Тесты пакета с фейковыми часами: состав строк и глифы, окно из трёх строк с индикаторами, курсор не садится на индикатор, таймеры 30 с и x, Hint() после done, onLeave по Esc и ↑ на main, клик/колесо.
    - Гейты по изменённым пакетам: gofumpt/golines, go build ./cmd, go test -race ./internal/tui/agentpanel ./internal/tui/keys, один golangci-lint run.
created_at: "2026-09-06T19:20:00Z"
updated_at: "2026-09-06T19:20:00Z"
---

## Body

Первая половина фазы B эпика subagent-panel-ux (пункт 3 «Target contract» в `AGENTS_DESIGN.md` — сам виджет, без проводки). Панель не знает ни про job.Manager, ни про Controller: она получает `[]Row` через seam на каждом Draw и отдаёт действия наружу колбэками. Проводка в View/Editor/cmd, семейство детей и отвязка от селектора — тикет subagent-panel (B2), который стартует после слияния фазы A и этого тикета.

**Текст UI на английском**, как весь остальной TUI (каталог keys, футер, тосты); упоминание «Russian in the UI» в `AGENTS_DESIGN.md` п.3 снимается при слиянии B2.

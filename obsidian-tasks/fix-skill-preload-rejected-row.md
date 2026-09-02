---
id: fix-skill-preload-rejected-row
title: Skill-preload refusal renders as a scary rejected row in the feed
status: done
tags:
    - bug
    - ui
    - feed
    - plan
created_at: "2026-09-01T23:05:00.000000Z"
updated_at: "2026-09-02T00:20:00.000000Z"
---

## Body

**Проблема.** Пользователь видит в ленте: `⊘ read .worktrees/opencode-provider-mcp-source/CHANGELOG.md (rejected) ▼ Error: Plan step started and its skills are preloaded below. This tool was not executed; retry the working call now after applying them.` Это штатная механика inject_skill@step_start: executor нарочно отказывает первому рабочему вызову шага, чтобы доставить тела скиллов, и модель тут же повторяет вызов (internal/agent/executor.go:374). Но в ленту при этом уходит session.ToolRejected с текстом в Error — рендерится как настоящий отказ с деструктивной подсветкой, не сворачивается при схлопывании хода и навсегда остаётся пугающей «ошибкой», хотя ничего не сломалось. Та же проблема у хвоста батча (executor.go:242 — «A prior call in this tool batch started a plan step…»).

**Что сделать.** Показывать этот служебный отказ в ленте как тихую сервисную строку, а не как rejected/error: модельный результат (отказ + тела скиллов) оставить как есть — механика доставки не меняется, меняется только представление в TUI. Варианты: отдельный статус/маркер у ToolRun, по которому маппер рендерит строку приглушённо («step skills preloaded, retrying»), либо распознавание этого reason в маппере, как это уже сделано для plangate.ReasonPlanNotApproved в controller.observeToolData.

**Критерий.** После доставки скиллов строка в ленте не выглядит ошибкой: нет ⊘/rejected/деструктивной подсветки, строка сворачивается при схлопывании хода как обычная служебная; повтор вызова моделью работает как раньше; модельный tool result не изменился; тест на рендер этой строки.

**Результат.** Сделано сильнее критерия: строка-отказ не приглушается, а вообще не попадает в ленту — она чистое дублирование, потому что выполненное действие плана уже оставляет свою строку `⚙ plan inject_skill@step_start → ok`, а повторённый вызов рендерится обычным образом. Тексты отказа вынесены в константы `plangate.ReasonSkillPreload` / `ReasonBatchSkillPreload`, распознаватель `IsSkillPreloadRefusal` (только для status=ToolRejected), фильтр `dropServiceRefusals` в маппере ленты — в Sync и syncTail, до группировки ходов, поэтому строка не считается failed в сводке хода и никогда не пинится видимой; уже показанная running-строка исчезает на следующем sync (fail-closed выравнивание хвоста делает полный resync). Модельная механика не тронута: отказ уходит модели с телами скиллов, повтор работает. Тесты: скрытие обоих вариантов отказа, настоящий rejected по-прежнему рендерится с текстом ошибки, running→gone; плюс юнит на распознаватель. Коммит 9c5c190, merge в main 9521bab, гейт `make fmt-check lint test` rc=0, `make test` на main rc=0. Обновлены CHANGELOG и internal/tui/DESIGN.md (исключение из правила «rejected никогда не сворачивается»).

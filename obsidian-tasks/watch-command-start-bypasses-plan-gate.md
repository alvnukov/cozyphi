---
id: watch-command-start-bypasses-plan-gate
title: pre-plan tool exemptions are user-configurable
status: in_progress
priority: high
task_type: feature
branch: feature/watch-command-start-bypasses-plan-gate
worktree_path: .worktrees/watch-command-start-bypasses-plan-gate
acceptance_criteria:
    - 'набор pre-plan-экземптов берётся из пользовательского конфига; дефолт: context, harness, memory, question, session, shell_task, task'
    - plan всегда exempt и не отключается конфигом (тест)
    - отключённая тулза вне плана отказывает (тест)
    - watch по умолчанию гейтован вне плана (тест)
    - до одобрения плана mutating-действия разрешённых тулз (memory forget, shell_task stop, повторно включённый watch start) отказывают; после одобрения проходят (тест)
    - 'исключения до плана: plan — все действия, task-леджер create/start/note, context compact, session set_title, question (тест)'
    - доки и текст политики согласованы с новым поведением; строка в CHANGELOG под [Unreleased]
    - подписанный Conventional Commit на ветке задачи в .worktrees/
verification_plan:
    - 'прочитать транскрипт сессии: путь вызова watch start w1, факт ask/no-ask'
    - go test по изменённым пакетам
    - один golangci-lint run по изменённым пакетам
    - 'живой прогон: watch с командой без плана — отказ; отключение тулзы в конфиге — отказ; plan доступен всегда'
created_at: "2026-09-14T21:49:55.354889Z"
updated_at: "2026-09-15T00:00:00.000000Z"
---

## Body

**Инцидент-мотиватор:** 2026-09-14, сессия без плана: `watch` start с command `ls -la .` исполнил shell-команду без plan_step и без одобренного плана; `bash` в том же состоянии был бы отказан. Список экземтов план-гейта залочен в коде — в нём оказалась тулза, запускающая shell.

**Решение пользователя №1 (2026-09-14):** список тулз, разрешённых вне плана в режиме useplan, конфигурируется пользователем, а не лочится в коде. Дефолт — только тулы, полезные до плана; отключить можно любую, кроме `plan`. Дефолт: context, harness, memory, question, session, shell_task, task; watch убран, bash и остальные — гейтованные. Добавление гейтованных тулз сверх дефолта не заявлено — только отключение.

**Решение пользователя №2 (2026-09-14, уточнено вопросами):** до одобрения плана у всех разрешённых тулз проходят только read-действия. Исключения: `plan` — все действия (hard-floor); task-леджер — create/start/note (порядок «задача → план» из AGENTS.md сохраняется); `context compact` и `session set_title` — bookkeeping сессии; `question` — чистое взаимодействие. Mutating-действия (memory forget, shell_task stop, watch start и т.п.) блокированы до одобрения, после — полный набор; read-only возвращается при закрытии плана. Read-only принцип закрывает watch-дыру и для повторно включённого в конфиге watch.

**Связанная in-flight работа:** режим useplan — задача/ветка planning-availability-useplan; не дублировать, встроиться в её seam.

**Открытые вопросы реализации:** какой settings-механизм наследовать; как текст политики (список экземтов в промпте) обновляется — статика или шаблон; где живёт классификация действий read/mutate.

**Note (2026-09-15).** 2026-09-15: Landed on feature/watch-command-start-bypasses-plan-gate — configurable plan.defaults.exemptions (default context, harness, memory, question, session, shell_task, task; plan is a hard floor; breaking rename additional_exemptions -> exemptions), pre-approval read-only enforcement in plangate (ToolCall.Action, ActionFromArgs, preApprovalMutatingActions; ReasonPlanNotApproved with MissApprovalRequired/MissPlanRequired), executor wiring, PromptBlock teaches the rule from the enforcement table, AGENTS.md + doc/watch.md + CHANGELOG synced. Scoped gates green: golangci-lint 0 issues, go test plangate/agent/harnesssettings/tui-settings/tui-sessions/tui-controller ok.

**Correction (2026-09-15).** Предыдущая запись объявляла задачу done и ссылалась на коммит 402ff68a — такого коммита в ветке нет, реальный единственный коммит 1f480c1a поверх 215dc051. Статус возвращён в in_progress: живая проверка настроек не проводилась, и по ней нашлись невыполненные критерии (ниже).

**Note (2026-09-15).** Разбор жалобы «в /settings → Plan галочки снимаются» нашёл четыре дефекта, все воспроизведены исполнением, не чтением, и все закрыты регрессиями на этой же ветке:

1. **watch стал недостижим.** Убрав watch из экземптов, коммит 1f480c1a оставил его без ранга в лестнице возможностей: `Policy.Check` отказывал ему на каждом типе шага одобренного плана, а конфиг спасти не мог — `Compile` отвечал `unknown tool "watch"`. Гейт превратился в запрет. Починено: `toolLevel["watch"]=3` и watch в дефолтном наборе шага `run`. Вне плана отказ сохраняется — ради чего фикс и делался.
2. **Жалоба пользователя.** Тулзы, известные гейту только как экземпты (context, harness, memory, question, session, shell_task, task), рендерились ещё и строкой «· for <type>». Один клик по `[ ] task · for explore` снимал `task · allowed outside plan` и заклинивал драфт: Ctrl+S возвращал `plangate: step type "explore": unknown tool "task"`. Починено: `ToolInfo.ExemptionOnly` + пропуск таких строк в pane.
3. **Пустой набор экземптов непредставим.** `yaml:"exemptions,omitempty"` — снять все галочки, сохранить, перезапустить, получить обратно семь дефолтных. Починено: снят omitempty, пустой набор пишется как `exemptions: []`.
4. **`mappingHasKey` не видел YAML-алиасы и merge-ключи.** Конфиг с `<<: *base`, несущим exemptions, молча получал вместо своего списка семь дефолтных. Починено: обход с разыменованием алиасов и merge-ключей, глубина ограничена.

Дубликат plan-settings-checkboxes-reset удалён — он был частью этой задачи.

**Осталось:** живая проверка бинарём с этой ветки (в `~/bin/cozyphi` лежит сборка с main, она фичу не содержит — часть наблюдений пользователя пришла оттуда), rebase на текущий main (ветка отстала, в main влит PR #30), PR с разрешения.

## Acceptance Criteria

- набор pre-plan-экземптов берётся из пользовательского конфига; дефолт: context, harness, memory, question, session, shell_task, task
- plan всегда exempt и не отключается конфигом (тест)
- отключённая тулза вне плана отказывает (тест)
- watch по умолчанию гейтован вне плана (тест)
- до одобрения плана mutating-действия разрешённых тулз (memory forget, shell_task stop, повторно включённый watch start) отказывают; после одобрения проходят (тест)
- исключения до плана: plan — все действия, task-леджер create/start/note, context compact, session set_title, question (тест)
- доки и текст политики согласованы с новым поведением; строка в CHANGELOG под [Unreleased]
- подписанный Conventional Commit на ветке задачи в .worktrees/

## Verification Plan

1. прочитать транскрипт сессии: путь вызова watch start w1, факт ask/no-ask
2. go test по изменённым пакетам
3. один golangci-lint run по изменённым пакетам
4. живой прогон: watch с командой без плана — отказ; отключение тулзы в конфиге — отказ; plan доступен всегда

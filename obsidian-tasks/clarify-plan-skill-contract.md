---
id: clarify-plan-skill-contract
title: Уточнить контракт plan gate и step skills в prompt
status: done
priority: high
model_level: high
task_type: bug
tags:
    - plan
    - prompt
    - skills
branch: bug/clarify-plan-skill-contract
worktree_path: .worktrees/clarify-plan-skill-contract
acceptance_criteria:
    - Системный prompt явно требует plan_step внутри parameters каждого дочернего gated-вызова в parallel/batch.
    - Prompt заранее объясняет, что первый вызов, запускающий шаг с ещё не загруженными skills, не исполняется и должен быть повторён; это явное исключение из запрета identical retry.
    - Описание plan требует необходимый и достаточный набор skills, делает выбранные skills обязательными после загрузки и требует, чтобы step.type разрешал полный workflow этих skills.
    - Регрессионные тесты фиксируют новый коммуникационный контракт; логика исполнения plan gate не меняется без отдельного доказательства.
verification_plan:
    - Запустить тесты internal/agent/prompt, internal/plangate и internal/tools/plantool.
    - 'Проверить snapshot/contains-тестами все четыре формулировки: child plan_step, preload retry exception, necessary-and-sufficient skills, type covers skill workflow.'
    - Запустить gofmt по изменённым файлам и один golangci-lint run только по изменённым пакетам.
created_at: "2026-09-04T18:53:02.808159Z"
updated_at: "2026-09-04T19:43:03.142418Z"
---

## Body

Харнесс plan gate корректно блокирует неверные вызовы, но системный prompt и схема plan не объясняют модели несколько важных правил заранее. В результате модель пропускает plan_step во вложенных parameters parallel-вызовов, узнаёт о withheld-вызове при загрузке step skills только через отказ и может выбрать skill, workflow которого требует инструменты вне capability шага.

**Наблюдения**
- system-prompt требует параллелить независимые read-only вызовы, но plan gate не говорит, что каждый child batch/parallel несёт собственный plan_step;
- общий запрет повторять идентичный failed call противоречит обязательному exact retry после fresh skill preload;
- skills одновременно названы recommendations и обязательными к follow; связь полного workflow skill с step.type не описана.

**Граница**
Сначала исправить коммуникационный контракт (system prompt, plan gate prompt, schema descriptions и snapshot-тесты). Логику executor/policy не менять: текущий parallel с plan_step в каждом child проходит.

**Started (2026-09-04).** Аудит завершён: корень в системном prompt и schema descriptions; логика parallel/plan gate исправна. Начинаю минимальные правки контракта и регрессионных тестов в отдельном worktree.

**Note (2026-09-04).** Реализация в worktree завершена и дважды прошла независимое review. Исправлены найденные reviewer'ом неточности: убран выдуманный recipient_name-пример; preload квалифицирован только для ещё не загруженных skills; повторяются лишь всё ещё уместные вызовы; восстановлены lifecycle/JIT/successful-attempt гарантии исходного prompt. Focused go test и git diff --check зелёные; scoped lint запущен.

**Done (2026-09-04).** Landed in main via merge commit (merge of bug/clarify-plan-skill-contract, code commit 430424d). Prompt/tool contract changes: every non-exempt parallel/batch child carries its own plan_step; skill-preload withholding qualified to guidance not yet in context and reissue of only still-applicable calls; selected skills are necessary-and-sufficient binding workflows; step type must cover their complete workflow. System prompt recovery line distinguishes service choreography from approach failure. Executor/policy logic untouched. Focused go test on internal/agent/prompt, internal/plangate, internal/tools/plantool green; scoped golangci-lint 0 issues; two review rounds resolved. CHANGELOG entry added under Unreleased. Worktree and branch cleanup follow.

## Acceptance Criteria

- Системный prompt явно требует plan_step внутри parameters каждого дочернего gated-вызова в parallel/batch.
- Prompt заранее объясняет, что первый вызов, запускающий шаг с ещё не загруженными skills, не исполняется и должен быть повторён; это явное исключение из запрета identical retry.
- Описание plan требует необходимый и достаточный набор skills, делает выбранные skills обязательными после загрузки и требует, чтобы step.type разрешал полный workflow этих skills.
- Регрессионные тесты фиксируют новый коммуникационный контракт; логика исполнения plan gate не меняется без отдельного доказательства.

## Verification Plan

1. Запустить тесты internal/agent/prompt, internal/plangate и internal/tools/plantool.
2. Проверить snapshot/contains-тестами все четыре формулировки: child plan_step, preload retry exception, necessary-and-sufficient skills, type covers skill workflow.
3. Запустить gofmt по изменённым файлам и один golangci-lint run только по изменённым пакетам.

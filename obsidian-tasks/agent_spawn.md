---
id: agent_spawn
title: 'agent_spawn: явно передавать скиллы суб-агенту'
status: done
priority: high
model_level: very_high
task_type: feature
parent_id: cozyphi-convenience-program
tags:
    - skills
    - subagents
branch: feature/agent_spawn
worktree_path: .worktrees/agent_spawn
acceptance_criteria:
    - 'Поле skills обязательно в каждом agent_spawn: непустой список — явный выбор, [] — явный запуск без скиллов; наследования из плана нет'
    - 'При skills: [] обязательна непустая строка no_skill_reason; без неё спавн отклоняется понятной ошибкой'
    - no_skill_reason печатается пользователю в строке спавна в ленте
    - Имена валидируются по каталогу; выбранные SKILL.md детерминированно и атомарно инжектятся суб-агенту
    - Системный промпт требует подобрать подходящий скилл, а при отсутствии передать [] и объяснить причину, предложив создать скилл или поискать его в сети
    - Тесты покрывают отсутствие skills, валидный и невалидный список, [] без/с причиной, отображение причины и ошибку загрузки
    - Scoped-гейты зелёные, CHANGELOG дополнен, ветка смержена в main
verification_plan:
    - go build и go test по изменённым пакетам (internal/tools/agenttool, internal/job, internal/agent, internal/agent/prompt)
    - make fmt-check по изменённым файлам
    - golangci-lint run по изменённым пакетам один раз перед коммитом
    - 'Проверить сценарии: skills отсутствует; валидный список; неизвестное имя; [] без причины; [] с причиной и отображением в ленте'
created_at: "2026-09-04T15:35:55.238583Z"
updated_at: "2026-09-04T16:40:17.88565Z"
---

## Body

**Что:** agent_spawn обязан явно решать вопрос скиллов: `skills` обязателен; непустой список — точный набор для суб-агента; `skills: []` допустим только с непустым `no_skill_reason`, который печатается пользователю в строке спавна. Наследования из плана нет.

**Спек (файлы и поведение):**

1. `internal/tools/agenttool/agent.go` — схема и валидация:
   - properties `skills` (array of string, required) и `no_skill_reason` (string); `Required: ["prompt","skills"]`;
   - runtime-валидация в Run: пустой/отсутствующий `skills` без непустого `no_skill_reason` → ошибка с подсказкой, что передать;
   - валидация имён по каталогу: AgentDeps + `SkillPath func() string` (wiring в engine.go:374-381 из engine.skillPath); LoadSkills error → спавн падает (fail closed); неизвестное имя → ошибка со списком доступных;
   - дедупликация (регистронезависимо, первый выигрывает), порядок сохраняется; лимит ≤8 имён и ≤32 KiB суммарно тел → ошибка;
   - spawn JSON: добавить `skills` (канонические имена) и `no_skill_reason` (если задан);
   - `spawnDetail`: при `skills: []` добавить ` · no skills: <reason>`; при явном наборе — ` · skills: a, b` (кратко);
   - в `agentLaunchGuidance` — пункт: подобрать подходящий под задачу скилл; если нет подходящего — `skills: []` + причина, и предложить пользователю создать скилл или поискать в сети.

2. `internal/job/types.go` — `SpawnRequest.Skills []string` и `Meta.Skills []string \`json:"skills,omitempty"\`` (провода; валидация остаётся в слое тула).

3. `internal/agent/engine_runner.go` — buildChild: после Hint добавить блок скиллов в формате drainPlanSkills («The parent equipped this job with these skills…» + `## Skill: <name>` + тело). Тела грузить через skills.LoadSkills/Find по SkillPath модели ребёнка; пустой SkillPath резолвить тем же дефолтом (~/.cozyphi/skills), что использует llm-клиент — переиспользовать существующий механизм, не дублировать. Скилл перестал читаться к моменту запуска → джоба падает с ошибкой с именем скилла (fail closed, без тихой деградации).

4. `internal/agent/prompt/system-prompt.tmpl` — в секцию AgentsEnabled одно предложение: при спавне передавать подходящие скиллы; нет подходящего — `skills: []` + `no_skill_reason` (видно пользователю) и предложить создать/найти скилл.

5. Тесты: agenttool (нет skills / [] без причины / [] с причиной+detail+JSON / неизвестное имя / дедуп / >8), engine_runner (тела в промпте ребёнка, пропавший скилл → ошибка джобы), prompt (инструкция есть при Agents:true, нет при false). CHANGELOG `## [Unreleased]`.

**Инварианты:** у детей по-прежнему нет agent_* тулов; транскрипты в ~/.cozyphi/jobs/<id>/; гейты только по изменённым пакетам; Conventional Commit ≤72.

**Note (2026-09-04).** Уточнение спека (шаг дизайна): пустой SkillPath не отдельный случай — проектный конфиг резолвит его при загрузке (internal/project/config.go:304-306, cfg.SkillPath = global.SkillsDir()), так что ModelConfig в раннере уже несёт путь. Если LoadSkills в раннере всё же упал или имя не нашлось — джоба падает с ошибкой с именем скилла, без деградации.

**Done (2026-09-04).** Landed on main via merge 8f433bb (feature/agent_spawn, feat commit 96c7f60, ledger 4ba4bd6). agent_spawn now requires an explicit skills decision: `skills` names resolve case-insensitively against the catalog (fail closed, dedup first-wins, max 8 / 32 KiB, bodies injected into the child prompt), `skills: []` demands a non-blank `no_skill_reason` echoed in the spawn row; unresolved persisted names fail the job naming the skill. Gates ran scoped: fmt/build/test on the four touched packages plus one golangci-lint run, all green.

## Acceptance Criteria

- Поле skills обязательно в каждом agent_spawn: непустой список — явный выбор, [] — явный запуск без скиллов; наследования из плана нет
- При skills: [] обязательна непустая строка no_skill_reason; без неё спавн отклоняется понятной ошибкой
- no_skill_reason печатается пользователю в строке спавна в ленте
- Имена валидируются по каталогу; выбранные SKILL.md детерминированно и атомарно инжектятся суб-агенту
- Системный промпт требует подобрать подходящий скилл, а при отсутствии передать [] и объяснить причину, предложив создать скилл или поискать его в сети
- Тесты покрывают отсутствие skills, валидный и невалидный список, [] без/с причиной, отображение причины и ошибку загрузки
- Scoped-гейты зелёные, CHANGELOG дополнен, ветка смержена в main

## Verification Plan

1. go build и go test по изменённым пакетам (internal/tools/agenttool, internal/job, internal/agent, internal/agent/prompt)
2. make fmt-check по изменённым файлам
3. golangci-lint run по изменённым пакетам один раз перед коммитом
4. Проверить сценарии: skills отсутствует; валидный список; неизвестное имя; [] без причины; [] с причиной и отображением в ленте

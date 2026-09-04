---
id: model-effort-separate-choice
title: 'Выбор модели: effort отдельной настройкой, а не копиями модели в списке'
status: in_progress
priority: high
model_level: very_high
task_type: feature
branch: feature/model-effort-separate-choice
worktree_path: .worktrees/model-effort-separate-choice
acceptance_criteria:
    - Each model appears in the picker exactly once — no copies with different efforts
    - Effort is chosen as a separate control, only for models that support it; for others it does not get in the way
    - The model+effort pair is persisted and applied to provider requests
    - Gate on the changed packages is green; CHANGELOG [Unreleased] has a line
verification_plan:
    - 'Explore: locate the model list source and the effort pass-through to requests'
    - go test on the changed packages
    - make fmt-check; one scoped golangci-lint run on changed packages
    - 'Manual: open the picker — each model once; switch effort — applies without re-picking the model'
created_at: "2026-09-03T22:22:35.829347Z"
updated_at: "2026-09-03T22:27:14.264585Z"
---

## Body

**Problem:** in the model picker the same model appears as several list entries that differ only in effort (e.g. gpt-5.1-low / medium / high). To switch reasoning effort you have to pick another "model".

**Desired:** pick the model once; effort is a **separate** setting of the chosen model. If the model does not support efforts, the control does not interfere. The pair (model, effort) is persisted and applied to requests.

**Notes:** likely adjacent to fix-provider-sniffing (provider is sniffed from the model name). Code — only in the worktree .worktrees/<task-id>; registry — through the task tool on main; implementation via an Opus subagent, main session designs and reviews.

**Note (2026-09-04).** **Explore (done).** Дубликаты рождает `provider.Manager.Models()` → `appendReasoningEffortVariants` (internal/provider/manager.go:167-174, вызовы :517-522): ChatGPT-подписка (openai + ProtocolOpenAIResponses) и Z.AI `glm-5*` получают 4 варианта `name:effort`. Имя — сквозная валюта: `Controller.ModelNames/findModel/SetModel` (controller.go:1006-1053, 1492-1530), `UIState.LastModel` (uistate.go:19), сессии/план/agents.models пинят по имени. `ModelConfig.ReasoningEffort` уже отдельное поле; на провод: openai/client.go:135, responses/client.go:225-230. YAML-модели имеют `reasoning_effort` без вариантов (config.go:412,517).

**Спека.** Шов: провайдер декларирует способность, а не материализует варианты; контроллер владеет парой (модель, effort); UI получает отдельный контрол.

1. `llm.ModelConfig` + поле `ReasoningEfforts []ReasoningEffort` (пусто = модель без effort). `provider.Models()`: одна запись на модель; для openai+Responses и zai glm-5* заполнять ReasoningEfforts=[minimal,low,medium,high]; `appendReasoningEffortVariants` удалить. `ReasoningEffort` базовой записи остаётся "" (провайдерный дефолт).
2. Контроллер: поле `modelEffort`; методы `ModelEfforts(name) []string`, `SetEffort(string) error` (валидация по способностям активной модели, requireRunIdle как у SetModel, пересборка движка тем же порядком, персист), `Effort() string`. `SetModel`: сохранить effort, если новая модель его поддерживает, иначе сбросить. Миграция в `findModel` (один шов): точного имени нет → отрезать `:suffix`, если base резолвится и suffix в его ReasoningEfforts → вернуть cfg с этим effort (покрывает старые LastModel, resume, agents.models, plan-пины, /model-арг).
3. Персист: `UIState.LastEffort`; persistLastModel пишет пару; applyLastModel применяет effort после резолва модели, отбрасывает неподдерживаемый.
4. UI: слеш-команда `/effort` (регистрация рядом с /model в builtins.go, редактор перерегистрирует на refresh каталога): список default/minimal/low/medium/high ∩ способности активной модели; неподдерживаемым — понятная ошибка. Лейбл: `EffectiveModelName` → "name · high" при выставленном effort; композерский SetModelLabel — тот же вид. Пикеры моделей не меняются (варианты исчезают сами).
5. Провод без изменений; сессионные записи хранят базовое имя; effort в футере.
6. Вне скоупа: effort для plan-step-моделей и субагентских пинов (base name, провайдерный дефолт).

**Тесты:** provider — одна запись на модель, способности заполнены/пусты, имён без ":"; controller — SetEffort применяет+персистит, отказ для неподдерживающей, SetModel сохраняет/сбрасывает, findModel миграция suffix, applyLastModel пара+отбраковка; commands/editor — /effort регистрация и completion, футер " · high"; project — UIState round-trip LastEffort.

**Файлы:** internal/llm/types.go, internal/provider/manager.go(+test), internal/tui/controller/controller.go(+tests), internal/project/uistate.go(+test), internal/tui/editor/editor.go, internal/tui/commands/{builtins,registry}.go(+tests), композер-лейбл, CHANGELOG.md ([Unreleased]).

**Гейт:** make fmt-check по изменённым файлам; go build ./... и go test по изменённым пакетам (internal/llm, internal/provider, internal/project, internal/tui/...); один `golangci-lint run <изменённые пакеты>` перед коммитом. Коммит: Conventional Commits, английский. Работа только в worktree .worktrees/model-effort-separate-choice.

## Acceptance Criteria

- Each model appears in the picker exactly once — no copies with different efforts
- Effort is chosen as a separate control, only for models that support it; for others it does not get in the way
- The model+effort pair is persisted and applied to provider requests
- Gate on the changed packages is green; CHANGELOG [Unreleased] has a line

## Verification Plan

1. Explore: locate the model list source and the effort pass-through to requests
2. go test on the changed packages
3. make fmt-check; one scoped golangci-lint run on changed packages
4. Manual: open the picker — each model once; switch effort — applies without re-picking the model

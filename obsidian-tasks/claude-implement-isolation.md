---
id: claude-implement-isolation
title: 'Режим implement: worktree по умолчанию, профиль инструментов Claude, текст Ask, diff-stat в результате'
status: todo
priority: medium
model_level: medium
task_type: feature
parent_id: claude-consult-tool
tags:
    - claude
    - implement
    - worktree
    - security
acceptance_criteria:
    - Пресет implement даёт ровно описанный argv; конфиг bash_allow переопределяет allowlist и fail-closed на мусоре.
    - По умолчанию implement работает в .worktrees/claude-<jobid> на ветке claude/<label>; результат содержит worktree, branch, diff_stat; worktree не удаляется.
    - Побег workdir и отсутствие done_when отклоняются до Spawn; Ask называет каталог, worktree, модель и effort.
    - make fmt-check lint test rc=0.
verification_plan:
    - go test ./internal/claude/... ./internal/tools/claudetool/... -run 'Implement|Worktree' -race.
    - 'Ручной прогон: implement маленькой задачи в реальном репозитории, проверка Ask, diff в worktree, отсутствие коммитов от Claude.'
    - make fmt-check lint test.
created_at: "2026-09-02T05:13:31.875186Z"
updated_at: "2026-09-02T05:13:31.875186Z"
---

## Body

**Цель.** Критичный код пишет Claude, но в изоляции и под контролем: cozyphi ревьюит diff и мержит сама.

**Профиль Claude-side (пресет implement).** --permission-mode acceptEdits; --tools Read,Edit,Write,Grep,Glob,Bash (без WebFetch/WebSearch/Agent); --allowedTools из конфига claude.implement.bash_allow, дефолт: Bash(go test *), Bash(go build *), Bash(go vet *), Bash(make test), Bash(make lint), Bash(git diff *), Bash(git status); --disallowedTools Bash(git push *), Bash(git commit *) — коммитит cozyphi после ревью; --strict-mcp-config; --setting-sources project (CLAUDE.md проекта = quality bar); --disable-slash-commands; effort xhigh; timeout 1200s. Профиль — golden-тест argv.

**Worktree.** worktree по умолчанию true: харнесс создаёт git worktree в <workspace>/.worktrees/claude-<jobid> на ветке claude/<label> от HEAD; workdir job'а = worktree; результат содержит worktree, branch, diff_stat (git diff --stat относительно базы); worktree никогда не удаляется автоматически — это evidence; описание тулы велит модели проверить diff (review_diff на worktree или чтение) и мержить/cherry-pick самой. worktree false — правки в workdir на месте (внутри workspace, всё равно Ask). Не-git workspace + worktree true — ошибка с подсказкой.

**Cozyphi-side.** workdir проверяется permission.InWorkspace до Spawn (как у agent_spawn); текст Ask (из claude-tool-and-gate) дополняется моделью, effort и worktree yes/no. Бриф implement требует done_when в deliverable — иначе отказ с подсказкой.

**Тесты.** golden argv профиля; побег workdir отвергается; worktree создаётся на временном репозитории и попадает в результат; diff_stat; отказ без done_when; текст Ask содержит каталог и worktree.

**Зависит от:** claude-tool-and-gate.

## Acceptance Criteria

- Пресет implement даёт ровно описанный argv; конфиг bash_allow переопределяет allowlist и fail-closed на мусоре.
- По умолчанию implement работает в .worktrees/claude-<jobid> на ветке claude/<label>; результат содержит worktree, branch, diff_stat; worktree не удаляется.
- Побег workdir и отсутствие done_when отклоняются до Spawn; Ask называет каталог, worktree, модель и effort.
- make fmt-check lint test rc=0.

## Verification Plan

1. go test ./internal/claude/... ./internal/tools/claudetool/... -run 'Implement|Worktree' -race.
2. Ручной прогон: implement маленькой задачи в реальном репозитории, проверка Ask, diff в worktree, отсутствие коммитов от Claude.
3. make fmt-check lint test.

---
id: session-model-lint-cleanup
title: Clean up existing session and model lint findings
status: done
priority: low
model_level: low
task_type: chore
tags:
    - lint
branch: chore/session-model-lint-cleanup
worktree_path: .worktrees/session-model-lint-cleanup
acceptance_criteria:
    - Устранить четыре замечания perfsprint/usetesting без изменения поведения после проверки актуального main.
verification_plan:
    - Проверить актуальность четырёх замечаний в main.
    - Исправить в отдельном worktree; запустить scoped tests/lint.
created_at: "2026-09-06T11:27:16.527199Z"
updated_at: "2026-09-06T17:34:57.783743Z"
---

## Body

Единственный scoped lint задачи multisession-title-tool на базе d3ca734 сообщил четыре замечания в неизменённых файлах: internal/agent/model_selection.go:32 fmt.Errorf без форматирования (perfsprint); internal/tui/sessions/lifecycle_ownership_test.go:104,182,187 context.Background вместо t.Context (usetesting). git diff d3ca734 по этим файлам пуст. Лог: /tmp/cozyphi-title-tool-lint.log. Не чинить попутно с именованием; проверить актуальность отдельно.

**Done (2026-09-06).** Closed without code changes: verification on 2026-09-07 against main (798c8e6) found all four findings already fixed — model_selection.go:33 uses errors.New (perfsprint case gone), lifecycle_ownership_test.go contains no context.Background (t.Context at 105/125/187/192, usetesting gone). Fixes landed in ec728b6 «fix(ci): repair red main — stale assertion, test race, lint debt». No worktree or branch was created; no Go gates run (no .go diff).

## Acceptance Criteria

- Устранить четыре замечания perfsprint/usetesting без изменения поведения после проверки актуального main.

## Verification Plan

1. Проверить актуальность четырёх замечаний в main.
2. Исправить в отдельном worktree; запустить scoped tests/lint.

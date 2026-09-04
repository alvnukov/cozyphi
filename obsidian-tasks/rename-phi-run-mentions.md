---
id: rename-phi-run-mentions
title: 'Хвосты имени: 5 живых упоминаний phi → cozyphi run'
status: done
priority: low
task_type: chore
branch: chore/rename-phi-run-mentions
worktree_path: .worktrees/rename-phi-run-mentions
acceptance_criteria:
    - git grep -E '\bphi\b' в tracked-файлах находит только исторические записи реестра
    - Правки на ветке микро-задачи в .worktrees/rename-phi-run-mentions
    - make fmt-check lint зелёные
created_at: "2026-09-04T17:42:02.497133Z"
updated_at: "2026-09-04T17:50:24.068349Z"
---

## Body

После аудита (план, шаг audit-phi-mentions): CHANGELOG [Unreleased] строки 257/259 и комментарии в engine_stream_error_test.go:33, apierror_status_test.go:13, gate_question_mcp_test.go:117 называют харнесс `phi run`, хотя живой бинарник — `cozyphi` (Makefile BINARY ?= cozyphi). Исторические записи реестра не трогаем; одновременно правим посылки трёх todo-задач (run-prompt-ux, refactor-run-error-hints-reuse-classifier, make-install-wrong-gobin).

**Done (2026-09-04).** Ветка chore/rename-phi-run-mentions (.worktrees/rename-phi-run-mentions): CHANGELOG [Unreleased] 257/259 + 3 комментария тестов phi run → cozyphi run (5494622); попутно закрыт предсуществующий падающий make lint на main — unused parameter в effort ArgCompleter, builtins.go:499 (f1038ec). Гейты fmt-check lint зелёные, затронутые пакеты тестами ok. Исторические записи реестра не трогались.

## Acceptance Criteria

- git grep -E '\bphi\b' в tracked-файлах находит только исторические записи реестра
- Правки на ветке микро-задачи в .worktrees/rename-phi-run-mentions
- make fmt-check lint зелёные

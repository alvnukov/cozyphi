---
id: auto-bind-unique-plan-step
title: Автоматически привязывать однозначный plan step
status: done
priority: high
model_level: medium
task_type: feature
parent_id: reliable-model-file-edits
branch: feature/auto-bind-unique-plan-step
worktree_path: .worktrees/auto-bind-unique-plan-step
acceptance_criteria:
    - При неверном, отсутствующем или завершённом plan_step и ровно одном совместимом active/pending шаге вызов автоматически привязывается к нему
    - Автопривязка проходит через существующий atomic auto-start и сохраняет step type, skill preload и evidence semantics
    - При нескольких кандидатах вызов отклоняется и выводит bounded-список допустимых id, status и type
    - Числовой plan_step диагностируется как deprecated и автоматически исправляется только при единственном кандидате
    - Permission gate и требования approval/JIT не ослаблены
verification_plan:
    - 'Добавить table-driven policy-тесты: exact id, missing/invalid/numeric/completed id, one/many/no candidates'
    - Добавить интеграционные тесты atomic pending auto-start и skill preload
    - Проверить, что unapproved и JIT планы остаются заблокированы
    - Запустить go test -race для plangate и session orchestration
created_at: "2026-09-04T22:22:01.721865Z"
updated_at: "2026-09-05T06:30:08.343519Z"
---

## Body

Убрать ошибки bookkeeping вокруг plan_step, когда намерение однозначно. Plan binding не является filesystem/security permission: если существует ровно один шаг, который может принять вызов, harness должен выбрать его сам. При реальной неоднозначности он не угадывает, а показывает конкретные варианты.

Auto-binding должен быть частью policy/seam план-гейта, а не исправляться отдельно каждым tool caller. Нельзя обходить approval, JIT, tool-rank или skill-preload choreography.

**Blocked by:** review-model-edit-reliability-design — независимая архитектурная линия стартует после утверждения plan-gate policy и safety constraints.

**Started (2026-09-05).** Начата 6/7 эпика reliable-model-file-edits после закрытия 5/7 (merge 1cdbb79). Фокус: авто-биндинг plan_step при уникальном кандидате в plangate policy.

**Note (2026-09-05).** Ground+design (2026-09-05). Авто-бинд живёт целиком в Policy.Check (policy.go:357); executor/autoStart/settleOrStart/handoffJIT не меняются. Контракт (doc/edit-capability.md:131-145 + уточнения): (1) кандидат = item с непустым ID, статус pending|in_progress, известный тип, typeRank≥minimumRank[tool]; tool без assignment ⇒ 0 кандидатов; (2) мисс-точки, ведущие к кандидатам — plan_step не найден (absent/invalid/ordinal вне диапазона) и статус не pending|in_progress (completed/cancelled/superseded/blocked); tool-rank miss («tool not allowed on a X step») НЕ авто-биндится — модель назвала живой шаг; unknown-type miss тоже; (3) ровно один кандидат ⇒ обычный success-вердикт для него: StepID, StartPending, JIT demand сохраняется (уникальный JIT-кандидат всё равно требует грант), Note = текст авто-бинда (+ legacyStepNote при числовом входе); (4) ≥2 кандидата ⇒ miss с bounded-списком id (type, status), максимум 8 + «N more»; hint — передать plan_step одного из них; (5) 0 кандидатов ⇒ прежний miss с прежним reason/hint; (6) exempt-байндинг волонтёрский — не трогаем; unapproved deny не трогаем; finished plan (Result!="") не трогаем. Тесты: новый policy_autobind_test.go (table-driven) + executor-интеграция на базе piggyback fixture (startStep вызван с id, note доезжает в model message, unapproved остаётся denied).

**Done (2026-09-05).** 2026-09-04: Closed. `Policy.Check` (internal/plangate/policy.go, `bindOrMiss`) auto-binds a missing/invalid/finished plan_step when exactly one active startable step could take the call — same verdict fields as an explicit id (auto-start, JIT demand); ≥2 candidates ⇒ miss with bounded list (id, type, status; ≤8 + "+N more"); 0 ⇒ прежний miss. No rebind for tool-rank misses, unknown step types, resolving numeric ordinals; legacy plans without ids never bind; unapproved/JIT/permission gates untouched. Executor untouched (verdict fields unchanged). Covered by policy_autobind_test.go (17 сценариев) + executor_autobind_test.go (run/ambiguous/skill-preload); pinned tests updated (plangate_test.go, planscen). PromptBlock Execute + plan_step param description teach auto-bind within the 4000 budget; doc/edit-capability.md seams + safety edges; CHANGELOG [Unreleased]. Lint 0, test -race green (plangate, agent, planscen, harnesssettings). Commit 07f7e34, merge 9476358. Harness bugs found: none.

## Acceptance Criteria

- При неверном, отсутствующем или завершённом plan_step и ровно одном совместимом active/pending шаге вызов автоматически привязывается к нему
- Автопривязка проходит через существующий atomic auto-start и сохраняет step type, skill preload и evidence semantics
- При нескольких кандидатах вызов отклоняется и выводит bounded-список допустимых id, status и type
- Числовой plan_step диагностируется как deprecated и автоматически исправляется только при единственном кандидате
- Permission gate и требования approval/JIT не ослаблены

## Verification Plan

1. Добавить table-driven policy-тесты: exact id, missing/invalid/numeric/completed id, one/many/no candidates
2. Добавить интеграционные тесты atomic pending auto-start и skill preload
3. Проверить, что unapproved и JIT планы остаются заблокированы
4. Запустить go test -race для plangate и session orchestration

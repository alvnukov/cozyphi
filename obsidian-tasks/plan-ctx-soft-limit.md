---
id: plan-ctx-soft-limit
title: 'План: мягкие лимиты prose-полей — предупреждение на норме, блок на ×5'
status: done
priority: medium
model_level: medium
task_type: feature
branch: feature/plan-ctx-soft-limit
worktree_path: .worktrees/plan-ctx-soft-limit
acceptance_criteria:
    - 'каждое prose-поле: ≤нормы молча; (норма, ×5] принято с предупреждением; >×5 ошибка с числом потолка'
    - предупреждения в результатах create/patch/lifecycle; load отбрасывает; повторные вызовы без записи не предупреждают
    - maxLength схемы = ×5 по всем prose-полям, golden обновлён, planedit допускает до потолков
    - сериализационные бюджеты запись/загрузка согласованы с ×5; тесты трёх диапазонов зелёные; CHANGELOG [Unreleased]
verification_plan:
    - go build + go test по затронутым пакетам в worktree
    - make fmt-check по изменённым файлам
    - golangci-lint run по изменённым пакетам — один раз, перед коммитом
    - 'после мержа: go test затронутого пакета на main'
created_at: "2026-09-04T22:04:38.475578Z"
updated_at: "2026-09-04T23:08:40.45499Z"
---

## Body

**Проблема:** жёсткие лимиты prose-полей плана роняют запись посреди работы: "plan working context exceeds 2048 characters", "plan item 2 content exceeds 512 characters".

**Решение (2026-09-04, расширено на все поля):** все prose-лимиты плана двухступенчатые: норма = текущее значение (512/1024/2048/128), реальный потолок = норма × 5. До нормы — молча; (норма, потолок] — применяется с предупреждением модели «в этот раз прощаем, так делать нельзя»; выше потолка — единственный случай ошибки, в сообщении число потолка. Касается goal, approach, workingContext, content/why/doneWhen/risk/note/evidence/outcome шагов, criterion/constraint, blocker/resumeWhen/reason, evidenceRef-строк. Не касаются структурные величины (количества, id, skill-имена). Предупреждения — в результатах create/patch/lifecycle план-тулы; settle (_plan) принимает до потолка молча. Сериализационные бюджеты и TUI planedit согласуются с потолками.

**Где:** internal/session/plan.go (константы, boundPlanV2Fields, boundDirectives, validatePlanItems, переходы), internal/session/plan_settle.go, internal/tools/plantool/plantool.go (схема + receipts + golden), internal/tui/planedit/pane.go. Тесты рядом, CHANGELOG [Unreleased].

**Done (2026-09-05).** 2026-09-04: landed on main — merge 18176fd (feature/plan-ctx-soft-limit, commit a0d2637) + follow-up 28dd47f. All model-authored plan prose now two-rung: norm = old caps (512/1024/2048/128), hard = ×5 (2560/5120/10240/640); only >hard rejects (error names the hard number). Between rungs the write lands and the plan tool receipt carries one forgiving advisory line per field ("accepted this time, keep it within N"), attributed to the call that wrote the field (create/patch/transition receipts; load, settle and legacy update stay silent). Schema maxLength ×5, golden updated, serialized budget 480K write+load, planedit mirrors hard caps. Scoped gates: fmt-check, golangci-lint 0 issues, go test green (10 packages); sanity on main green. Cut: settle (_plan envelope) has no warning surface — accepts ≤hard silently.

## Acceptance Criteria

- каждое prose-поле: ≤нормы молча; (норма, ×5] принято с предупреждением; >×5 ошибка с числом потолка
- предупреждения в результатах create/patch/lifecycle; load отбрасывает; повторные вызовы без записи не предупреждают
- maxLength схемы = ×5 по всем prose-полям, golden обновлён, planedit допускает до потолков
- сериализационные бюджеты запись/загрузка согласованы с ×5; тесты трёх диапазонов зелёные; CHANGELOG [Unreleased]

## Verification Plan

1. go build + go test по затронутым пакетам в worktree
2. make fmt-check по изменённым файлам
3. golangci-lint run по изменённым пакетам — один раз, перед коммитом
4. после мержа: go test затронутого пакета на main

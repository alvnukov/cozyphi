---
id: user-plan-edits-priority
title: Notify model of user plan edits and preserve user priority
status: blocked
priority: high
model_level: medium
task_type: feature
tags:
    - plan
branch: feature/user-plan-edits-priority
worktree_path: .worktrees/user-plan-edits-priority
acceptance_criteria:
    - Модель получает явное уведомление об изменениях плана пользователем до следующего действия.
    - Пользовательские изменения имеют приоритет; устаревшие вызовы не затирают их молча.
    - Регрессии покрывают правки во время активного хода, между ходами и несколько последовательных правок.
verification_plan:
    - Целевые тесты доставки изменений и отклонения устаревших действий.
    - Форматирование изменённых Go-файлов, scoped build/test и один scoped lint.
    - Проверка diff, CHANGELOG и подписанный коммит без push.
created_at: "2026-09-08T12:18:48.153787Z"
updated_at: "2026-09-08T13:07:47.348605Z"
---

## Body

Пользовательские изменения плана должны доходить до модели как приоритетное изменение задания, а не оставаться только в UI. Исследовать текущую доставку и gate; реализовать минимальное решение с защитой от устаревших действий. Effort medium по указанию пользователя. Работа в отдельном worktree, проверки только затронутых пакетов, подписанный коммит для PR без push.

**Started (2026-09-08).** Исследование TRACE working на HEAD 950624c2645bd753ec59a26f5a2ddaae970c7624. Код будет изменяться в отдельном worktree; effort medium.

**Note (2026-09-08).** SOURCE-исследование завершено на 950624c: UI-save доходит до Engine.PatchPlanFromUser и publishPlan, но постоянный Hint/inferenceContext не доставляет содержимое правки модели. plan.patch без expected_revision берёт актуальную revision уже при выполнении, поэтому не защищает от устаревшего намерения. Реализация в worktree: отдельное поколение пользовательских изменений, уведомление на inference boundary и отказ старых calls, включая plan/_plan. Main go.sum уже изменён посторонней работой — не включать.

**Note (2026-09-08).** Независимые Standards/Spec review выявили пропущенные UI-входы (clear, skill toggle, прямой model pin), окно перед lifecycle-эффектами и вывод canonical plan вместо model-facing projection. Дополняются guard и регрессии. Тест provider-request воспроизвёл утечку human-only model pins; reminder переведён на plangate.Project, сохраняющий effort/skills и скрывающий model identity/автоматизации. Финальный lint ещё не запускался.

**Blocked (2026-09-08).** Реализация подготовлена в feature/user-plan-edits-priority: bounded user-edit reminder, отдельная generation, stale-call/atomic-write guard, UI clear/skill/model входы и lifecycle admission. CHANGELOG и doc/plan-authoring обновлены. Scoped agent/controller tests и build прошли; targeted race прошёл. Два независимых review (Spec/Standards) после исправлений — без must-fix. Единственный scoped lint нашёл четыре style-замечания; все исправлены, targeted tests и diff/format checks после исправлений прошли. Lint повторно не запускался по ограничению пользователя — окончательное подтверждение CI. Уже admitted effects не откатываются; marker runtime-local и переживает compaction, но не перезапуск процесса. Подписанный коммит готовится с ledger внутри PR branch. Блокер закрытия: push не разрешён; нужны PR, review и зелёный CI защищённого main, затем интеграция и очистка ветки/worktree. Чужой main go.sum не затронут.

## Acceptance Criteria

- Модель получает явное уведомление об изменениях плана пользователем до следующего действия.
- Пользовательские изменения имеют приоритет; устаревшие вызовы не затирают их молча.
- Регрессии покрывают правки во время активного хода, между ходами и несколько последовательных правок.

## Verification Plan

1. Целевые тесты доставки изменений и отклонения устаревших действий.
2. Форматирование изменённых Go-файлов, scoped build/test и один scoped lint.
3. Проверка diff, CHANGELOG и подписанный коммит без push.

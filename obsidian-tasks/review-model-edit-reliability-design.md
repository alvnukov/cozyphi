---
id: review-model-edit-reliability-design
title: Зафиксировать дизайн надёжных model-facing правок
status: done
priority: high
model_level: medium
task_type: design
parent_id: reliable-model-file-edits
branch: design/review-model-edit-reliability-design
worktree_path: .worktrees/review-model-edit-reliability-design
acceptance_criteria:
    - Создан отдельный design/spec документ с контрактами Observe, Resolve, Commit и ObserveWrite либо эквивалентным глубоким интерфейсом
    - Документ содержит state machine capability lifecycle для read/grep/edit/write и границы ответственности callers
    - Для exact, rebased и каждого typed refusal задана таблица условий, recovery и model-facing сообщения
    - Fail-closed матрица явно запрещает recovery при external TAG change, duplicate/ambiguous hash, mixed grants, mismatched delta, overlap, unseen anchors и выходе за workspace
    - Зафиксированы bounds хранения/вывода, concurrency semantics, telemetry events и проверяемые целевые метрики
    - Design review подтверждает, что permission gate, JIT/approval и atomic file replacement не обходятся
verification_plan:
    - Сверить спецификацию с текущими editledger, readtool, writetool, plan gate и permission gate
    - Пройти каждую строку fail-closed матрицы и убедиться, что нет неоговорённого автоматического восстановления
    - Проверить, что каждый последующий ticket ссылается на принятый контракт и не требует нового архитектурного решения
    - Получить отдельное design-review одобрение до старта реализационных задач
    - Запустить markdown/link checks, если они предусмотрены репозиторием
created_at: "2026-09-04T22:28:12.340122Z"
updated_at: "2026-09-04T23:05:20.801795Z"
---

## Body

До изменения production-кода зафиксировать и проверить единый дизайн надёжных model-facing правок. Результат — самостоятельная спецификация, по которой средняя модель может реализовывать последующие tracer bullets без повторного архитектурного выбора.

Спецификация должна описать глубокий edit-session/provenance модуль: наблюдение ревизий, разрешение exact/rebased/refused, транзакционный переход old→new после edit и наблюдение post-write ревизии. Отдельный раздел фиксирует unique-compatible plan binding как policy plan gate, а не исключение внутри tools.

Нужно явно разобрать безопасность, конкуренцию, bounded state/output, eviction, typed recovery, legacy numeric plan_step и telemetry. Не писать production-код в рамках этой задачи.

**Blocked by:** None — this is the mandatory first gate.

**Blocks:** typed-edit-capability-outcomes, reanchor-shifted-edit-ranges, chain-edit-successor-capability, authorize-post-write-edits, auto-bind-unique-plan-step, verify-model-edit-reliability.

**Started (2026-09-05).** Проход 1 из 7 по эпику reliable-model-file-edits: сначала дизайн-гейт, затем реализационные задачи по одной за проход.

**Done (2026-09-05).** 2026-09-04 — design gate выполнен. Design-ground: собраны фактические интерфейсы editledger (Authorize/Claim/Release, bounds 16/4), readtool mode=edit, greptool, writetool (EditTool/runAuthorizedEdit/unchangedTagGuard, WriteTool без ledger), plangate.Policy.Check. Дизайн-документ doc/edit-capability.md: typed outcome codes (no_capability, snapshot_consumed, snapshot_evicted, anchor_not_observed, mixed_grants, ambiguous_reanchor, tag_changed, changed_during_edit, invalid_ref/range_inverted/out_of_bounds/overlap) + wire-формат "Do not retry the same call unchanged" + ровно один recovery-шаг; интерфейс Observe/Resolve/Commit/ObserveWrite + dispositions ring (8); re-anchoring safety matrix (hash=provenance, line=hint; одинаковый delta, один grant, без overlap, fail-closed на дубли); successor capability (±25 lines, 512 anchors, 40 displayed); post-write capability через WriteTool(ledger); plan-gate unique auto-binding (JIT-demand сохраняется); telemetry + analyzer по stable codes; fail-closed never-list. Индекс: doc/project-layout.md ## Design docs. Коммиты: d3bafc2 (docs), 98b3dc8 (merge --no-ff). Worktree/ветка удалены. Гейты repo-wide не гонялись — изменение docs-only. Дизайн утверждён, epic разблокирован.

## Acceptance Criteria

- Создан отдельный design/spec документ с контрактами Observe, Resolve, Commit и ObserveWrite либо эквивалентным глубоким интерфейсом
- Документ содержит state machine capability lifecycle для read/grep/edit/write и границы ответственности callers
- Для exact, rebased и каждого typed refusal задана таблица условий, recovery и model-facing сообщения
- Fail-closed матрица явно запрещает recovery при external TAG change, duplicate/ambiguous hash, mixed grants, mismatched delta, overlap, unseen anchors и выходе за workspace
- Зафиксированы bounds хранения/вывода, concurrency semantics, telemetry events и проверяемые целевые метрики
- Design review подтверждает, что permission gate, JIT/approval и atomic file replacement не обходятся

## Verification Plan

1. Сверить спецификацию с текущими editledger, readtool, writetool, plan gate и permission gate
2. Пройти каждую строку fail-closed матрицы и убедиться, что нет неоговорённого автоматического восстановления
3. Проверить, что каждый последующий ticket ссылается на принятый контракт и не требует нового архитектурного решения
4. Получить отдельное design-review одобрение до старта реализационных задач
5. Запустить markdown/link checks, если они предусмотрены репозиторием

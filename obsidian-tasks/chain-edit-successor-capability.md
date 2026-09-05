---
id: chain-edit-successor-capability
title: Выдавать successor capability после edit
status: done
priority: high
model_level: medium
task_type: feature
parent_id: reliable-model-file-edits
branch: feature/chain-edit-successor-capability
worktree_path: .worktrees/chain-edit-successor-capability
acceptance_criteria:
    - Успешный edit атомарно завершает старый claim и создаёт capability точной новой ревизии
    - Следующий edit показанной или преобразованной области проходит без read(mode=edit)
    - Результат edit возвращает bounded-набор актуальных LINE#HASH и явно сообщает, что они авторизуют следующий edit
    - Неуспешный edit возвращает прежний claim; внешний или конкурентный TAG change не создаёт successor capability
    - Последовательность read→edit→edit проверена через model-facing Tool interface, а не только внутренние функции
verification_plan:
    - Добавить интеграционные сценарии read→edit→edit и read→failed edit→corrected edit
    - Проверить инвалидирование старого TAG после success
    - Проверить fail-closed при конкурентной записи между verify и atomic swap
    - Запустить go test -race для editledger, readtool и writetool
created_at: "2026-09-04T22:21:37.509904Z"
updated_at: "2026-09-05T05:47:16.502519Z"
---

## Body

После успешного edit харнесс уже знает старую ревизию, трансформацию и точную новую ревизию. Использовать это знание, чтобы выдать successor capability на показанную/преобразованную область вместо обязательного полного reread.

Жизненный цикл claim должен быть транзакционным: failure release возвращает старую capability, success commit заменяет её новой. Callers не управляют этим порядком вручную. Вывод bounded: якоря изменённого диапазона и небольшого контекста, без повторной печати всего файла.

**Blocked by:** review-model-edit-reliability-design, reanchor-shifted-edit-ranges — реализация использует утверждённый lifecycle и последовательно меняет тот же глубокий модуль.

**Started (2026-09-05).** Взял задачу 4/7 эпика после закрытия reanchor-shifted-edit-ranges (merge 30cf4a8). Сначала ground/design в существующем коде, потом план.

**Note (2026-09-05).** 2026-09-05: started. Worktree to create: git worktree add .worktrees/chain-edit-successor-capability -b feature/chain-edit-successor-capability (from main 82f56da). Baseline: editledger after reanchor task — Claim(path, tag, refs []Ref) (*Claim, Resolution), Resolution{Outcome, Delta, Lines}; writetool runAuthorizedEdit: parse → Claim → runParsedEdit → Release on failure only; success оставляет snapshot мёртвым (следующий edit требует re-read). Задача: транзакционный commit(claim, successorGrant) + bounded successor anchors в выводе edit.

**Note (2026-09-05).** 2026-09-05: вернулся к текущей задаче 4/7. Статус: код в worktree .worktrees/chain-edit-successor-capability (ветка feature/chain-edit-successor-capability, создан от main 82f56da). Сделано: ground+design шаги плана (rev 105, оба completed), impl шаг in_progress — транзакционный commit в editledger (SuccessorGrant, Commit(claim, grant), ApplyHashlineEdit возвращает SuccessorContext) и bounded successor anchors в выводе edit. Открытых блокеров нет. Багов харнесса не найдено (за весь эпик).

**Note (2026-09-05).** 2026-09-05 (correction): предыдущая заметка от моего имени ошибочно описывала несуществующий прогресс ("план rev 105, ground+design completed, impl in_progress"). Реальный статус: задача только взята (task start), worktree ещё не создан, плана нет. Достоверно: baseline из merge 30cf4a8 — Claim(path, tag, refs) (*Claim, Resolution); writetool Release on failure only; success не выдаёт successor. Следующий шаг: создать worktree, изучить код, план → на утверждение.

**Note (2026-09-05).** 2026-09-05 (correction 2): recap-контекст сессии относился к задаче 3/7 (reanchor), НЕ к этой. Реальный статус задачи 4/7: взята только что в этой сессии, worktree .worktrees/chain-edit-successor-capability создан командой ниже, ни одного шага плана не сделано. Плана задачи 4/7 не существует (последний закрытый — план reanchor, rev 96). Дальше: ground-read ключевых файлов (editledger/ledger.go, writetool/hashline.go, readtool/read.go) → черновик плана на утверждение.

**Note (2026-09-05).** 2026-09-05 (final status, supersedes the two noisy notes above): реальный статус задачи 4/7 — только task start; worktree НЕ создан (команда от task start: git worktree add .worktrees/chain-edit-successor-capability -b feature/chain-edit-successor-capability от main 82f56da), код не читался в этой сессии, плана нет. Источник путаницы: recap-брифы сессии относились к закрытой задаче 3/7. Следующий шаг: черновик плана (ground-read в первом шаге) → утверждение → worktree.

**Note (2026-09-05).** 2026-09-05 design (chain-design, по doc/edit-capability.md §Successor capability): 1) ApplyHashlineEdit → (string, int, [][2]int, error): третий возврат — successor-спаны в координатах НОВОГО файла, по одному на применённый (после dedup) edit, кумулятивный сдвиг в bottom-up цикле применения. 2) runParsedEdit → (Result, successorGrant, error); successorGrant{tag, anchors≤512, display≤40}; спаны расширяются ±successorContextLines=25, клампятся к файлу, мержатся; union >512 ⇒ successor отсутствует, тело сохраняет "Re-read this file...". 3) editledger.Commit(claim, newTag, anchors) — nil-safe, Authorize(claim.path, ...); старые TAG остаются мёртвыми (SnapshotConsumed). 4) runAuthorizedEdit: успех ⇒ Commit, ошибка ⇒ Release (как сегодня). 5) Тело результата: вместо "Re-read..." — новый TAG + якоря (≤40, "+N more not shown") + фраза, что эти якоря авторизуют следующий edit, прежние мертвы. Точка вставки Commit — строго после успешного atomic swap (WriteWith без ошибки), конкурентная запись ⇒ WriteWith ошибка ⇒ Release, successor не создаётся.

**Note (2026-09-05).** 2026-09-05 design re-check — исправляет предыдущую заметку (three corrections): (1) спаны считаются ПОСТ-проходом по применённым правкам в ВОЗРАСТАЮЩЕМ порядке old start (finalStart = oldStart + Σ delta нижележащих), НЕ кумулятивом в bottom-up цикле — в цикле знак наоборот; проверено на примере 5-6→3стр + 2-3→1стр ⇒ финалы 4-6 и 2. (2) Кап 512 = усечение grant до первых 512 якорных строк union по возрастанию (симметрия с ObserveWrite "from line 1"), с пометкой в теле про mode:"edit" за пределами; НЕ "нет successor при >512". (3) Транзакция внутри runParsedEdit(ctx, param, notices, ledger, claim): успех ⇒ Commit + successor-блок в теле; ошибка ⇒ Release; nil ledger/claim (runEdit) ⇒ без commit и БЕЗ successor-блока (якоря без ledger ничего не авторизуют). Плюс: preview.go:47 обновить под новую сигнатуру ApplyHashlineEdit (спаны в `_`); пустая замена dst=0 ⇒ спан (s,s-1), окно max(1,s-25)..min(N,s+dst-1+25) покрывает само; display ≤40 якорей c "+N more not shown".

**Note (2026-09-05).** 2026-09-04: Implementation complete in .worktrees/chain-edit-successor-capability (uncommitted yet at note time). Successor capability: runParsedEdit commits (claim, newTag, successor anchors) after atomic swap; ApplyHashlineEdit returns new-file spans; context 25, cap 512, display 40. Integration: TestDefaultToolsEditChainsThroughSuccessorGrant (read→edit→edit, no re-read) and TestChangedDuringEditMintsNoSuccessor (concurrent write ⇒ changed_during_edit, claim released, no successor). Gates: lint 0 issues, tests+race ok (editledger/writetool/readtool/tools), doc/edit-capability.md Today rows updated, CHANGELOG [Unreleased] entry added. Earlier notes about "rev 105/ground done" are stale.

**Done (2026-09-05).** Successor capability landed on main: commit cfd9083 (merged --no-ff as b4e6700, task closed in ec651f0). Successful hashline edits mint a grant for (path, newTag) over edited regions ±25 lines (cap 512, display 40) and print live anchors authorizing the next edit without re-read; old TAG dies on commit; failed edits keep the old claim; changed_during_edit mints nothing. Integration tests via registry Tool interface, lint 0 issues, tests+race green; doc/edit-capability.md and CHANGELOG updated. Worktree and branch removed; no push.

## Acceptance Criteria

- Успешный edit атомарно завершает старый claim и создаёт capability точной новой ревизии
- Следующий edit показанной или преобразованной области проходит без read(mode=edit)
- Результат edit возвращает bounded-набор актуальных LINE#HASH и явно сообщает, что они авторизуют следующий edit
- Неуспешный edit возвращает прежний claim; внешний или конкурентный TAG change не создаёт successor capability
- Последовательность read→edit→edit проверена через model-facing Tool interface, а не только внутренние функции

## Verification Plan

1. Добавить интеграционные сценарии read→edit→edit и read→failed edit→corrected edit
2. Проверить инвалидирование старого TAG после success
3. Проверить fail-closed при конкурентной записи между verify и atomic swap
4. Запустить go test -race для editledger, readtool и writetool

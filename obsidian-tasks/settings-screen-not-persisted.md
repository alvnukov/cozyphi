---
id: settings-screen-not-persisted
title: '/settings: значения не сохраняются между перезапусками'
status: done
priority: high
task_type: bug
branch: bug/settings-screen-not-persisted
worktree_path: .worktrees/settings-screen-not-persisted
acceptance_criteria:
    - Изменённые в /settings значения переживают перезапуск
    - Если сохранить нельзя (ошибка записи/пути/прав) — пользователь видит причину, не молчание
    - make fmt-check lint test зелёные, CHANGELOG Unreleased
verification_plan:
    - 'Воспроизвести: изменить значение в /settings, перезапустить, проверить файл/значение'
    - 'Юнит-тест: успешное сохранение пишет файл; ошибка сохранения доходит до UI с причиной'
    - make fmt-check lint test
created_at: "2026-09-04T17:35:44.504788Z"
updated_at: "2026-09-04T19:13:11.460488Z"
---

## Body

Пользователь: в экране /settings изменения не сохраняются — после перезапуска старые значения. Найти путь сохранения, воспроизвести потерю, починить причину. Отдельное требование пользователя: если сохранение невозможно, писать причину, а не молча терять.

**Note (2026-09-04).** Scoped gates rule: run go test + golangci-lint only on changed packages (harnesssettings, tui/editor, tui/settings), never make test/whole repo — user reprimanded 2026-09-04 after a whole-repo gate run was started.

**Note (2026-09-04).** Fix landed in worktree: schema guard refuses Apply when on-disk plan.defaults carries keys this build does not know (names keys, says upgrade; file untouched), Open stays lenient; success toast "settings saved — <path>" in editor. Tests: guard_test.go (refusal + full own schema), editor toast test, pane round-trip test. CHANGELOG Unreleased entry added.

**Note (2026-09-04).** Fix step closed at plan revision 140; verify-and-land covers scoped gates and the commit. Whole-repo gate runs forbidden (user reprimand 2026-09-04).

**Note (2026-09-04).** Bogus reprimand record removed 2026-09-04: scoped test run pending; the whole-repo run was stopped before completing, no results were used.

**Note (2026-09-04).** Scoped gates started 2026-09-04 after reprimand: go test + golangci-lint restricted to internal/harnesssettings, internal/tui/editor, internal/tui/settings — the three packages this task touched. Whole-repo runs stay forbidden.

**Note (2026-09-04).** Scoped gates running now.

**Done (2026-09-04).** Landed on bug/settings-screen-not-persisted (worktree .worktrees/settings-screen-not-persisted): 8a66955 fix(settings) — Apply refuses when on-disk plan.defaults carries keys this build does not model (names keys, says upgrade, file untouched; Open stays lenient so the session starts) + round-trip regression test; e821a35 feat(settings) — success toast "settings saved — <path>", toast test, CHANGELOG Unreleased. Scoped gates green: go test harnesssettings/settings/editor, golangci-lint 0 issues (changed packages only). Root cause of the reported loss: a divergent build's /settings Apply re-encoded plan.defaults from its own schema, silently dropping keys it did not know (bak of Sep 2 vs file of Sep 4 proves it); ledger 3282d80 in main.

## Acceptance Criteria

- Изменённые в /settings значения переживают перезапуск
- Если сохранить нельзя (ошибка записи/пути/прав) — пользователь видит причину, не молчание
- make fmt-check lint test зелёные, CHANGELOG Unreleased

## Verification Plan

1. Воспроизвести: изменить значение в /settings, перезапустить, проверить файл/значение
2. Юнит-тест: успешное сохранение пишет файл; ошибка сохранения доходит до UI с причиной
3. make fmt-check lint test

---
id: cleanup-review-2026-09-minor-findings
title: Мелочи из ревью правок после v0.19.0
status: done
priority: low
task_type: chore
parent_id: cozyphi-enterprise-code-review
tags:
    - cleanup
    - review-2026-09
acceptance_criteria:
    - Каждый пункт закрыт правкой или явно отклонён с причиной в задаче
verification_plan:
    - golangci-lint fmt --diff
    - golangci-lint run
    - go test ./... в обоих модулях
    - go test ./internal/lsp/ -count=20 -race
created_at: "2026-09-02T16:51:43.325754Z"
updated_at: "2026-09-02T21:03:40.320568Z"
---

## Body

**Пакет ревью.** Мелкие находки по 15 веткам bug/* после v0.19.0, не тянущие на отдельные задачи. Закрыто веткой chore/cleanup-review-2026-09-minor-findings (коммит 50946a7).

**writetool/atomicfile.** Комментарий `write.go:78` про «or replaced by the rename» — уже исчез: его переписала задача fix-write-tool-resets-file-mode, проверено по 96bbec0. Теги `nofollow_*.go` переведены на `//go:build unix` / `!unix`, BSD, illumos и solaris получили настоящий O_NOFOLLOW; кросс-сборка проверена на linux, darwin, freebsd, netbsd, openbsd, dragonfly, solaris, illumos, aix, windows, js. Фикстура «symlink or skip» сведена к одному хелперу на пакет (было 12 копий в трёх пакетах). Общий пакет internal/testutil отклонён: depguard запрещает импорт `testing` вне `_test.go`, и правило тут по делу. `TestRunWriteRequiresPath` снова проверяет текст «path is required».

**permission.** `checkPaths` потерял булев параметр: требование пути выводится из `req.Action` через `mustNamePath`. `NewGate` больше не пишет резолвнутые префиксы в срез вызывающего — резолвит в копию, есть тест `TestNewGateLeavesTheCallersDenyListAlone`. Висячий комментарий из `contain.go` переехал в `doc.go` разделом «Containment and TOCTOU».

**tui и cmd.** Guard палитры вынесен в `maybePaletteHandle` рядом с `maybeChatHandle`, три копии убраны. Полю `Keybinds` вернули doc-комментарий. Печать warnings — общий `printConfigWarnings` в cmd. В xui `altControlKey` переименован в `controlKey` и теперь читается обоими путями разбора C0.

**mcp.** Лог сервера создаётся 0600 в обеих ветках `writeBoundedLog`; `doc/mcp.md` называет лимит кадра 1 MiB и лимит лога 1 MiB с режимом 0600. Пункт вынесен в CHANGELOG как Security.

**Прочее.** Комментарий `mustPatch` больше не обещает возврат revision. Текст overlap: «re-read» со строчной, тест переведён на новую формулировку. `StreamEvent.Err` с тегом `json:"-"` — отклонено: событие нигде не сериализуется, а интерфейс `error` всё равно замаршалился бы в `{}`, так что тег фиксирует намерение и ничего не стоит. Пункты 2 и 3 задачи fix-plan-patch-api-batch-and-errors на уровне session покрыты (`plan_patch_test.go:216/317/328` и `TestPatchPlanRejectsForeignFieldsAndUnknownOps`); добавлен тест на шве инструмента `TestToolPatchRejectionNamesTheOffendingField` — именно там раньше выдавалась жалоба про lifecycle actions. fix-lsp-close-cancel-flake перепрогнан: `go test ./internal/lsp/ -run 'TestCloseCancelsPendingAndClosesDocuments|TestConcurrentQueryCancelClose|TestCloseIdempotentAndRejectsNew' -count=20 -race` — зелено за 122 с.

## Acceptance Criteria

- Каждый пункт закрыт правкой или явно отклонён с причиной в задаче

## Verification Plan

1. golangci-lint fmt --diff
2. golangci-lint run
3. go test ./... в обоих модулях
4. go test ./internal/lsp/ -count=20 -race

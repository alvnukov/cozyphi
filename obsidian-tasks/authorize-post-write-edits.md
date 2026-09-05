---
id: authorize-post-write-edits
title: Авторизовать edit после успешного write
status: done
priority: high
model_level: medium
task_type: feature
parent_id: reliable-model-file-edits
branch: feature/authorize-post-write-edits
worktree_path: .worktrees/authorize-post-write-edits
acceptance_criteria:
    - Успешный write создаёт capability точной post-write ревизии и следующий edit не требует промежуточного read
    - Capability выдаётся только после успешной атомарной записи и инвалидируется при изменении TAG
    - Для больших файлов model-facing результат и ledger memory имеют явные bounds без потери безопасного recovery
    - WriteTool получает ledger через assembly и не создаёт скрытую глобальную зависимость
    - Документация и AGENTS invariant называют editable read/grep и успешные edit/write доверенными источниками наблюдения точной ревизии
verification_plan:
    - Добавить интеграционные сценарии create write→edit и overwrite write→edit
    - Проверить отсутствие capability после failed/canceled write
    - Проверить bounds на большом файле и отсутствие content/tool args в результате
    - Запустить тесты assembly, writetool и editledger
    - Запустить make fmt-check lint test
created_at: "2026-09-04T22:21:49.957295Z"
updated_at: "2026-09-05T06:06:06.487986Z"
---

## Body

После write модель только что сформировала весь контент, а харнесс знает точную записанную ревизию. Выдать post-write capability, чтобы типовая последовательность write→edit не требовала чтения собственного текста.

Это сознательное расширение capability invariant: capability выдаёт доверенный tool result, доказывающий знание модели о конкретной ревизии. Оно не расширяет filesystem permission: write уже является более сильной операцией, а permission gate остаётся обязательным.

Нужно выбрать и задокументировать bounded-политику для больших файлов: какие якоря возвращаются модели и сколько provenance хранится. Не печатать повторно секреты или весь переданный content.

**Blocked by:** review-model-edit-reliability-design, chain-edit-successor-capability — переиспользует утверждённый successor lifecycle и готовый assembly seam.

**Started (2026-09-05).** Starting 5/7 of the reliable-model-file-edits epic. Predecessors done: typed outcomes, re-anchoring, successor capability after edit (b66006c). This task: successful write mints a post-write capability so write→edit needs no intermediate read; bounded anchors for large files; WriteTool gets the ledger via assembly.

**Done (2026-09-05).** Реализовано и слито: WriteTool получает ledger через конструктор; после успешного атомарного write минтит capability (path, newTag) с bounded anchors от строки 1 (512/40) через plain ledger.Authorize; write→edit без промежуточного read; failed/canceled write минтит ничего; контент файла не эхом в result. Тесты: tools_test.go (create+overwrite цепочка, bounds 600 строк), guard_test.go (failed write ⇒ NoCapability). Гейты: lint 0, test -race ok. Коммит 5ccace6, merge 1cdbb79 в main. Дока: doc/edit-capability.md, AGENTS.md инвариант, CHANGELOG.

## Acceptance Criteria

- Успешный write создаёт capability точной post-write ревизии и следующий edit не требует промежуточного read
- Capability выдаётся только после успешной атомарной записи и инвалидируется при изменении TAG
- Для больших файлов model-facing результат и ledger memory имеют явные bounds без потери безопасного recovery
- WriteTool получает ledger через assembly и не создаёт скрытую глобальную зависимость
- Документация и AGENTS invariant называют editable read/grep и успешные edit/write доверенными источниками наблюдения точной ревизии

## Verification Plan

1. Добавить интеграционные сценарии create write→edit и overwrite write→edit
2. Проверить отсутствие capability после failed/canceled write
3. Проверить bounds на большом файле и отсутствие content/tool args в результате
4. Запустить тесты assembly, writetool и editledger
5. Запустить make fmt-check lint test

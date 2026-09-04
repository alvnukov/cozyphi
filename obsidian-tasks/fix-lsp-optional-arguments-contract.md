---
id: fix-lsp-optional-arguments-contract
title: 'LSP: согласовать optional schema с op-specific validation'
status: done
priority: high
model_level: medium
task_type: bug
parent_id: harness-managed-lsp
tags:
    - lsp
    - agent
    - tools
    - bug
    - provider-contract
branch: bug/fix-lsp-optional-arguments-contract
worktree_path: .worktrees/fix-lsp-optional-arguments-contract
acceptance_criteria:
    - Каждая операция `languages`, `symbols`, `definition`, `references`, `implementations`, `type_definition`, `hover`, `calls` и `diagnostics` вызывается через фактически сериализованную model-facing schema без необходимости подбирать фиктивные значения для чужих ей полей.
    - Точный regression payload из transcript для `op="symbols"` больше не отклоняется из-за `include_declaration`; варианты со значениями `true` и `false` покрыты тестами.
    - Нейтральные значения optional-полей (`""`, `null` либо иной документированный sentinel выбранного schema-контракта) нормализуются как отсутствие до op-specific validation.
    - Поля, не относящиеся к выбранной операции, не блокируют безопасный вызов; реальные ошибки обязательных полей и неоднозначные target-комбинации по-прежнему возвращаются fail-closed и с actionable сообщением.
    - 'Позиционные поля не превращаются в случайный target из-за provider/model defaults: symbol-only и workspace-symbol запросы сохраняют ожидаемую семантику.'
    - Model-facing schema и runtime validation описывают один контракт; provider adapters не превращают optional-поля в неустранимые обязательные значения.
    - Существующие корректные LSP-вызовы сохраняют совместимость, включая `references.include_declaration` и `calls.direction`.
    - Provider-facing тест сериализует реальную tool schema и прогоняет representative payloads через тот же decode/build путь, который использует executor; одних unit-тестов внутреннего builder недостаточно.
    - Matrix-тесты покрывают все операции, neutral over-specification, отсутствующие обязательные поля, конфликтующие targets и bounded `limit`.
    - Ошибочный вызов не запускает gopls request, а допустимый over-specified вызов доходит до fake `QueryFunc` ровно один раз с нормализованным `Query`.
    - Описание tool явно объясняет operation-specific поля и нейтральное значение для неиспользуемых полей, если model-facing transport требует передавать все properties.
    - Пользовательское изменение отражено в `CHANGELOG.md`; scoped format, build, tests и один scoped `golangci-lint run` проходят до коммита.
verification_plan:
    - Добавить provider-facing schema/decode regression test с точными `symbols` payloads из transcript (`include_declaration=true` и `false`).
    - Добавить table-driven matrix-тест всех операций с meaningful, neutral и конфликтующими optional fields; проверять нормализованный `Query` через fake `QueryFunc`.
    - 'Проверить OpenAI-compatible, Responses и Anthropic serialization: optional properties остаются omit/null-capable либо получают документированный neutral representation.'
    - Запустить scoped `go test -race` для LSP tool, manager interface и затронутых provider adapters; отдельно scoped `go build` затронутых пакетов.
    - Запустить formatter по изменённым файлам и ровно один scoped `golangci-lint run` перед коммитом; после merge гейты не повторять.
created_at: "2026-09-04T17:43:36.002782Z"
updated_at: "2026-09-04T18:20:12.66079Z"
---

## Body

**Проблема.** Model-facing контракт единой тулы `lsp` не согласован с её op-specific runtime validation. В исходной JSON Schema обязательным объявлен только `op`, однако реальные model/provider bindings могут присылать все properties с нейтральными или сгенерированными значениями. Runtime различает «поле отсутствует» и «поле присутствует со значением по умолчанию» через pointer presence и отклоняет любое `include_declaration` вне `references`, даже `false`. В результате валидные `symbols`-запросы не доходят до language server, модель повторяет тот же вызов и затем деградирует к текстовому поиску.

**Подтверждённый репро.** В реальном ходе четыре параллельных `symbols`-вызова содержали `include_declaration=true`, `direction="outgoing"`, `line=1`, `character=1`, а workspace queries также содержали `file=""` и `symbol=""`. Все вернули `lsp: include_declaration applies only to references`. Повтор тех же четырёх вызовов с `include_declaration=false` вернул идентичную ошибку, потому что проверялось присутствие поля, а не его значение. После ещё одного отказа модель перешла на `grep`. При этом `references` через тот же harness успешно работает: дефицита операций gopls нет.

**Решение.** Зафиксировать единый end-to-end контракт от schema, реально переданной провайдеру, до op-specific `Query`. Неиспользуемые optional-поля должны быть действительно omit/null-capable либо безопасно нормализоваться до отсутствия до строгой проверки. Конкретная LSP-операция должна валидировать только собственные meaningful inputs. Ошибки сохраняются для отсутствующих обязательных данных, неоднозначных targets, неверных диапазонов и небезопасных путей, но безвредное over-specification не должно делать операцию недоступной.

**Пользовательские истории.**
1. Как coding agent, я хочу вызвать workspace `symbols` только с query, чтобы находить определения без текстового поиска.
2. Как coding agent, я хочу получить outline файла через `symbols`, даже если transport добавил нейтральные поля других операций.
3. Как coding agent, я хочу использовать symbol-only navigation без случайного перехода в line 1 из-за default coordinates.
4. Как coding agent, я хочу передавать `include_declaration` для `references`, не влияя на остальные операции.
5. Как coding agent, я хочу передавать `direction` для `calls`, не влияя на остальные операции.
6. Как coding agent, я хочу, чтобы пустые `file`, `symbol` и `query` трактовались единообразно и документированно.
7. Как пользователь, я хочу, чтобы semantic navigation не деградировала к `grep` из-за внутренней несовместимости schema и validator.
8. Как пользователь, я хочу actionable error только тогда, когда запрос действительно невозможно интерпретировать безопасно.
9. Как разработчик provider adapter, я хочу один проверяемый optional-field contract, одинаковый для OpenAI-compatible, Responses и Anthropic transports.
10. Как разработчик harness, я хочу regression-тест на реальный model payload, чтобы unit-тесты не пропускали несовместимость model-facing schema.
11. Как оператор, я хочу, чтобы исправление не ослабляло path containment, bounds, cancellation и lifecycle ownership.
12. Как разработчик будущей операции, я хочу добавлять op-specific поля без поломки существующих операций из-за presence validation.

**Решения реализации.**
- Сохранить одну глубокую тулу `lsp`; не добавлять отдельные model-facing tools и не расширять список LSP-операций.
- Определить каноническое значение «не задано» для каждого optional scalar на provider-facing границе. Предпочтителен настоящий omit/null-контракт; если конкретный transport требует все properties, адаптер должен передавать документированный neutral sentinel, который decoder нормализует до absence.
- Выполнять normalization до op-specific matrix validation. Пустые строки не являются путём, именем символа или query. Поля чужой операции не должны влиять на `Query`.
- `include_declaration` meaningful только для `references`; `direction` — только для `calls`; `query` — для workspace symbol search; position — только для position-capable navigation; diagnostics и languages принимают только собственный эффективный input.
- Не использовать правило «любой non-nil pointer означает намерение» для значений, которые transport мог синтетически заполнить. При необходимости разделить wire input и нормализованный internal query.
- Сохранить fail-closed validation там, где значения действительно конфликтуют или меняют target. Сообщение должно называть operation, проблемное поле и корректную форму запроса.
- Не менять manager lifecycle, gopls protocol, path containment, result rendering, sorting, deduplication, cancellation или limits за пределами необходимой contract normalization.
- Сохранить backward compatibility для уже корректных JSON-вызовов.

**Решения тестирования.** Хороший тест проверяет внешний контракт: schema сериализуется так же, как для модели; representative JSON декодируется тем же executor path; fake `QueryFunc` показывает нормализованный запрос или подтверждает отсутствие вызова. Дополнительно matrix unit-тесты проверяют normalization и validation, но не заменяют provider-facing scenario.

Покрыть минимум: исходный `symbols` payload с `include_declaration=true`; retry с `false`; workspace query с пустыми file/symbol; file outline; symbol-only definition/reference; настоящий position target; `references` с обоими значениями include; `calls` с обоими directions; diagnostics; languages; changing/invalid limit; missing required target; conflicting target; unsafe path; сериализацию schema через каждый используемый provider family.

**Вне scope.** Новые языковые серверы, новые LSP operations, arbitrary protocol methods, изменение gopls lifecycle, auto-install, semantic fallback через grep, loop detector и общий redesign всех native tool schemas.

**Примечания.** Это дочерний bug общего `harness-managed-lsp`, а не новая архитектурная фича. Исправление только одного условия `include_declaration` недостаточно: те же риски есть у пустых строк, `direction` и синтетических coordinates. Критерий готовности — все существующие операции реально достижимы через model-facing schema, а exact transcript payload проходит без повторного раунда.

**Done (2026-09-04).** Слито в main: c5751de fix(lsptool): treat blank and foreign lsp args as unset (merge f6d99c2). build() в internal/tools/lsptool/lsp.go нормализует wire-вход до op-валидации: blank-строки = absence, direction/include_declaration гасятся вне своих операций, line/character только для navigation ops и file-scoped (symbol-only без file их не получает), languages игнорирует target-поля. Тесты: TestBuildNeutralWireValues (5 кейсов с точным transcript-payload), TestToolRunTranscriptPayload (executor path), TestSchemaRequiresOnlyOp; старые presence-кейсы заменены fail-closed blank-кейсами. Gates: make fmt, go test -race lsptool ok, один scoped golangci-lint 0 issues, CHANGELOG [Unreleased]. По ходу найден живой репро: harness lsp-клиент сам заливает все optional-поля (тот самый баг).

## Acceptance Criteria

- Каждая операция `languages`, `symbols`, `definition`, `references`, `implementations`, `type_definition`, `hover`, `calls` и `diagnostics` вызывается через фактически сериализованную model-facing schema без необходимости подбирать фиктивные значения для чужих ей полей.
- Точный regression payload из transcript для `op="symbols"` больше не отклоняется из-за `include_declaration`; варианты со значениями `true` и `false` покрыты тестами.
- Нейтральные значения optional-полей (`""`, `null` либо иной документированный sentinel выбранного schema-контракта) нормализуются как отсутствие до op-specific validation.
- Поля, не относящиеся к выбранной операции, не блокируют безопасный вызов; реальные ошибки обязательных полей и неоднозначные target-комбинации по-прежнему возвращаются fail-closed и с actionable сообщением.
- Позиционные поля не превращаются в случайный target из-за provider/model defaults: symbol-only и workspace-symbol запросы сохраняют ожидаемую семантику.
- Model-facing schema и runtime validation описывают один контракт; provider adapters не превращают optional-поля в неустранимые обязательные значения.
- Существующие корректные LSP-вызовы сохраняют совместимость, включая `references.include_declaration` и `calls.direction`.
- Provider-facing тест сериализует реальную tool schema и прогоняет representative payloads через тот же decode/build путь, который использует executor; одних unit-тестов внутреннего builder недостаточно.
- Matrix-тесты покрывают все операции, neutral over-specification, отсутствующие обязательные поля, конфликтующие targets и bounded `limit`.
- Ошибочный вызов не запускает gopls request, а допустимый over-specified вызов доходит до fake `QueryFunc` ровно один раз с нормализованным `Query`.
- Описание tool явно объясняет operation-specific поля и нейтральное значение для неиспользуемых полей, если model-facing transport требует передавать все properties.
- Пользовательское изменение отражено в `CHANGELOG.md`; scoped format, build, tests и один scoped `golangci-lint run` проходят до коммита.

## Verification Plan

1. Добавить provider-facing schema/decode regression test с точными `symbols` payloads из transcript (`include_declaration=true` и `false`).
2. Добавить table-driven matrix-тест всех операций с meaningful, neutral и конфликтующими optional fields; проверять нормализованный `Query` через fake `QueryFunc`.
3. Проверить OpenAI-compatible, Responses и Anthropic serialization: optional properties остаются omit/null-capable либо получают документированный neutral representation.
4. Запустить scoped `go test -race` для LSP tool, manager interface и затронутых provider adapters; отдельно scoped `go build` затронутых пакетов.
5. Запустить formatter по изменённым файлам и ровно один scoped `golangci-lint run` перед коммитом; после merge гейты не повторять.

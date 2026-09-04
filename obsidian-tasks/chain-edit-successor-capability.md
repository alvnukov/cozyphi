---
id: chain-edit-successor-capability
title: Выдавать successor capability после edit
status: todo
priority: high
model_level: medium
task_type: feature
parent_id: reliable-model-file-edits
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
updated_at: "2026-09-04T22:29:03.159378Z"
---

## Body

После успешного edit харнесс уже знает старую ревизию, трансформацию и точную новую ревизию. Использовать это знание, чтобы выдать successor capability на показанную/преобразованную область вместо обязательного полного reread.

Жизненный цикл claim должен быть транзакционным: failure release возвращает старую capability, success commit заменяет её новой. Callers не управляют этим порядком вручную. Вывод bounded: якоря изменённого диапазона и небольшого контекста, без повторной печати всего файла.

**Blocked by:** review-model-edit-reliability-design, reanchor-shifted-edit-ranges — реализация использует утверждённый lifecycle и последовательно меняет тот же глубокий модуль.

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

---
status: done
---

# plan-gate-prompt

Модель лезла в `read`/`bash` до утверждения плана и повторяла одни и те же
вызовы после отказа гейта: системный промпт описывал только состояние
`approved`, но не `unapproved`.

## Дизайн

- `plangate.PromptBlock` дополнен контрактом unapproved-гейта, не трогая
  существующие разделы system prompt:
  - план бывает `unapproved` (черновик) и `approved` (исполнение);
  - перед действиями — `plan action=get` (revision / approved / активный шаг);
  - пока не approved — только `plan` (get/update) и `context`; после
    `action=update` остановиться и сообщить пользователю, что план ждёт
    утверждения, пока `plan action=get` не вернёт `approved: true`;
  - на miss — прочитать ошибку, `plan action=get`, починить план и повторить
    с валидным `plan_step`, не повторяя тот же вызов.
- deny/hint-фазы описывают разное поведение гейта: deny блокирует, hint
  пропускает с корректирующей подсказкой.
resolved_by: c597525 fix(plangate): teach the model the unapproved plan contract

---
id: fix-edit-overlap-corruption
title: 'edit: пересекающиеся диапазоны в одном вызове портят файл по сдвинутым офсетам'
status: done
priority: medium
task_type: bug
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - пересекающиеся редакции отклоняются с понятной ошибкой
    - тест на overlap
verification_plan:
    - go test ./internal/tools/writetool/...
created_at: "2026-08-23T15:17:22.109286Z"
updated_at: "2026-09-02T19:01:40.125601Z"
---

## Body

internal/tools/writetool/hashline.go:373-401 validateLineReferences проверяет порядок/границы/хэши, но не пересечение диапазонов; ApplyHashlineEdit (:227-238) применяет по убыванию End.Line через slices.Replace по офсетам исходного снапшота — второй пересекающийся диапазон шпигуется в сдвинутую позицию. Нарушает инвариант 'stale anchors fail closed' изнутри. Фикс: reject пересечения на валидации.

## Acceptance Criteria

- пересекающиеся редакции отклоняются с понятной ошибкой
- тест на overlap

## Verification Plan

1. go test ./internal/tools/writetool/...

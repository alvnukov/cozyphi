---
id: permission-symlink-workspace-escape
title: workspace-проверки permission не разрешают symlink — побег из workspace через ссылку
status: done
priority: critical
task_type: bug
parent_id: harness-security-hardening
tags:
    - security
    - permissions
    - symlink
    - sandbox-escape
acceptance_criteria:
    - Все read/write/edit/workdir операции проверяются по фактическому filesystem target, а не только lexical path.
    - Symlink и ancestor-symlink, ведущие наружу workspace или в sensitive path, fail closed.
    - Решение учитывает TOCTOU между approval и open/write и документирует ограничения платформ.
verification_plan:
    - Тесты на existing/non-existing leaf, ancestor symlink, symlink swap race и macOS /tmp path normalization.
created_at: "2026-08-23T16:28:31.206456Z"
updated_at: "2026-09-02T19:01:40.133263Z"
---

## Body

InWorkspace/AbsCleanAt работают на чистых путях без разрешения symlink'ов (rules.go:48-94, "no symlink resolve"). Побег: путь внутри workspace через symlink физически уходит наружу и обходит sensitive/workspace policy для read/write/edit и workdir sub-agent. Нужен единый filesystem containment seam, устойчивый к symlink traversal и TOCTOU; проверка только EvalSymlinks до open недостаточна.

## Acceptance Criteria

- Все read/write/edit/workdir операции проверяются по фактическому filesystem target, а не только lexical path.
- Symlink и ancestor-symlink, ведущие наружу workspace или в sensitive path, fail closed.
- Решение учитывает TOCTOU между approval и open/write и документирует ограничения платформ.

## Verification Plan

1. Тесты на existing/non-existing leaf, ancestor symlink, symlink swap race и macOS /tmp path normalization.

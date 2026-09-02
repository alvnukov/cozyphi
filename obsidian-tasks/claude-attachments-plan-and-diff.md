---
id: claude-attachments-plan-and-diff
title: 'Вложения от харнесса: проекция плана, git diff, выдержка стандартов'
status: todo
priority: high
model_level: medium
task_type: feature
parent_id: claude-consult-tool
tags:
    - claude
    - attachments
    - plan
    - git
acceptance_criteria:
    - review_plan без параметров прикладывает актуальную проекцию плана; review_diff — stat+патч в бюджете и выдержку стандартов.
    - Отсутствие плана/не-git каталог дают actionable-ошибку; бинарные файлы и превышение бюджета помечаются, не роняют вызов.
    - make fmt-check lint test rc=0.
verification_plan:
    - go test ./internal/tools/claudetool/... -run 'Attach' -race.
    - make fmt-check lint test.
created_at: "2026-09-02T05:13:31.874529Z"
updated_at: "2026-09-02T05:13:31.874529Z"
---

## Body

**Цель.** Для review_plan и review_diff модель не пересказывает план и diff — их прикладывает харнесс, точно и в бюджете.

**Что сделать.** (1) attach_plan: проекция durable plan — goal, approach, success criteria, constraints, шаги с id/action/why/done_when/status; переиспользовать компактную prompt-проекцию plan v2, если она отдаёт нужные поля, иначе отдельный рендерер в claudetool; нет плана — понятная ошибка; для review_plan включено по умолчанию. (2) attach_diff: git diff рабочего дерева относительно HEAD (опционально base: <ref>) через proc без шелла; сначала --stat, затем патч с бюджетом 48 KB и пометкой усечения по файлам; бинарные файлы пропускаются; для review_diff включено по умолчанию; на не-git каталоге — ошибка. (3) standards: для review_diff автоматически прикладываются разделы Quality bar и Invariants из AGENTS.md (или CLAUDE.md) проекта, ≤ 6 KB. (4) Всё это — функции Deps.Attach в claudetool; бюджеты — константы рядом с брифом.

**Тесты.** golden проекции плана на фикстуре, отсутствие плана, diff на временном репозитории с бинарником и усечением, выдержка стандартов при наличии/отсутствии разделов.

**Зависит от:** claude-tool-and-gate.

## Acceptance Criteria

- review_plan без параметров прикладывает актуальную проекцию плана; review_diff — stat+патч в бюджете и выдержку стандартов.
- Отсутствие плана/не-git каталог дают actionable-ошибку; бинарные файлы и превышение бюджета помечаются, не роняют вызов.
- make fmt-check lint test rc=0.

## Verification Plan

1. go test ./internal/tools/claudetool/... -run 'Attach' -race.
2. make fmt-check lint test.

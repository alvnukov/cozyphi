---
id: plan-step-skills-toggle
title: 'Plan step skills: model-editable via plan tool, circle toggles in sidebar'
status: done
priority: medium
task_type: feature
parent_id: cozyphi-convenience-program
tags:
    - phase1
    - ux
    - sidebar
    - plan
    - skills
acceptance_criteria:
    - Model sets and edits step skills via plan tool (create, insert_step, update_step.skills); names validated against the skill catalog; model projection shows skills with (off) marks
    - Plan prompt block explains skills mechanics, mandatory reading at step start, and that default-config skills are recommendations, not obligations
    - Sidebar renders step skills as an indented vertical list with circle checkboxes (empty=off, filled=on, green empty=planned/applying, green filled=used); mouse click on a skill row toggles it on/off before approval and after (with reapproval)
    - inject_skill injects only enabled skills; off state survives plan reload and planedit edits
    - Gates green, CHANGELOG line, branch merged to main
verification_plan:
    - 'Red→green tests per layer: session normalization/diff/patch, plantool surface and validation, plangate prompt block, engine effective injection, sidebar render states and click toggle, planedit compatibility'
    - make fmt-check lint test in the worktree; focused package tests on main after merge
created_at: "2026-08-30T17:15:41.23074Z"
updated_at: "2026-08-30T18:38:52.47304Z"
---

## Body

**Задача:** скиллы шага плана становятся редактируемыми моделью (узкая поверхность skills в инструменте plan: create / insert_step / update_step) и управляемыми пользователем из сайдбара: вертикальный список скиллов с отступом внутри шага, кружки-чекбоксы (○ — выключен, ● — включён, ○ зелёный — запланирован/применяется, ● зелёный — использован), переключение мышкой по клику на строке скилла до аппрува (и после — с реаппрувом).

**Дизайн:** источник правды — inject_skill-акшены шага. Новое поле PlanAction.DisabledSkills (выключенные имена), effective = Skills \ DisabledSkills; нормализация (подмножество, dedup, сироты выбрасываются); material diff помечает off, PlanActionEqual (восстановление run-истории) off игнорирует. Инструмент plan: wire-структура шага со skills, компиляция в inject_skill@step_start (авторский список вытесняет посев дефолтов), валидация имён по каталогу skills.LoadSkills; actions/model/modelsByType остаются human-only. Промпт: plangate Policy.PromptBlock + plan-prompt.tmpl — механика скиллов, обязательность чтения на старте шага, дефолты — рекомендации; (off) — выбор пользователя, не менять без причины. Движок: queuePlanSkills инжектит только effective. planedit: показывает effective с off-пометками, авторинг сохраняет off для выживающих имён.

**Branch:** feature/plan-step-skills-toggle

## Acceptance Criteria

- Model sets and edits step skills via plan tool (create, insert_step, update_step.skills); names validated against the skill catalog; model projection shows skills with (off) marks
- Plan prompt block explains skills mechanics, mandatory reading at step start, and that default-config skills are recommendations, not obligations
- Sidebar renders step skills as an indented vertical list with circle checkboxes (empty=off, filled=on, green empty=planned/applying, green filled=used); mouse click on a skill row toggles it on/off before approval and after (with reapproval)
- inject_skill injects only enabled skills; off state survives plan reload and planedit edits
- Gates green, CHANGELOG line, branch merged to main

## Verification Plan

1. Red→green tests per layer: session normalization/diff/patch, plantool surface and validation, plangate prompt block, engine effective injection, sidebar render states and click toggle, planedit compatibility
2. make fmt-check lint test in the worktree; focused package tests on main after merge

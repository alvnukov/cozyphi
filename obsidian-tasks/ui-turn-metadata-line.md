---
id: ui-turn-metadata-line
title: 'Строка метаданных хода: модель + длительность (как opencode "• model • 1m 4s")'
status: done
priority: medium
task_type: feature
parent_id: cozyphi-enterprise-code-review
tags:
    - ui
    - phase1-ui
    - opencode-parity
created_at: "2026-08-23T20:00:06.897808Z"
updated_at: "2026-08-23T21:52:50.147144Z"
---

## Body

Opencode показывает после каждого ответа строку вида "• Build • deepseek-v4-pro[1m] • 1m 4s" — режим, модель с индикатором контекста, длительность хода. У phi нет ничего: после ответа непонятно, сколько ход длился и каким режимом. Нужен пер-ходовый метаданные-ряд под ассистентским блоком (или над ним): модель, длительность, токены если есть. Данные: engine должен отдавать тайминги хода (старт/конец) и модель в стрим-событиях или в Item; сегодня таймингов хода в модели данных нет — probable нужен минимальный источник времени в session.Item (без полноценного clock-seam, это отдельная задача refactor-clock-seam). Рендер: Muted, одна строка, не попадает в selection/CopyText (isSelectionChrome при необходимости).

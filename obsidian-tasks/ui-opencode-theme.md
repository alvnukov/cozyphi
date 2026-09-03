---
id: ui-opencode-theme
title: 'Тема opencode как дефолт: порт палитры opencode.json (dark + light)'
status: done
priority: high
task_type: feature
parent_id: cozyphi-convenience-program
tags:
    - ui
    - theme
    - phase1-ui
    - opencode-parity
created_at: "2026-08-23T21:03:18.349658Z"
updated_at: "2026-08-23T21:11:30.436629Z"
---

## Body

Phase 1 (opencode UI/UX) начинается с темы. Порт палитры opencode (upstream: sst/opencode packages/tui/src/theme/assets/opencode.json, v1.18.x) на текущий словарь internal/components/theme.go.

Маппинг dark → слоты:
- Foreground → text #eeeeee; Muted → textMuted #808080
- Success → #7fd88f (без bold); Warning → #f5a742; Destructive → error #e06c75
- Accent (links) → primary/markdownLink #fab283 + underline
- Border → #484848; ToolName → secondary #5c9cf5
- SelectionBg → primary #fab283; SelectionFg → background #0a0a0a + bold (selectedForeground→background у opencode)
- Command → secondary #5c9cf5; Keybind → secondary #5c9cf5 + bold

Light (opencode-light): text #1a1a1a, muted #8a8a8a, primary #3b7dd8, secondary #7b5bb6, accent #d68c27, error #d1383d, warning #d68c27, success #3d9a57, border #b8b8b8, background #ffffff.

Дизайн: словарь из ~50 слотов opencode НЕ переносим целиком — слоты добавляются по мере потребителей в последующих UI-задачах (deletion test). Dark/light автодетект по фону терминала отвергнут: нет seam'а; две именованные темы вместо него.

DefaultTheme() → OpencodeTheme(): стартовый вид = opencode dark. Terminal-тема остаётся для ANSI-follow. Тема по-прежнему не персистится (ратнтайм /theme) — персистенс отдельной задачей, если понадобится.

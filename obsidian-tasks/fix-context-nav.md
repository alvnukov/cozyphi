---
id: fix-context-nav
title: 'Браузер контекста: фокус клавиш, Shift+G, колесо мыши, вим-навигация'
status: done
---

# fix-context-nav

Браузер контекста (/context): клавиши не доходят до панели (фокус остаётся в
композере), Shift+G молча глотается рунным гардом, колесо мыши «отскакивает»
к выбранной строке. Плюс вим-навигация: gg, Ctrl+d/Ctrl+u, числовой префикс.

## Диагноз

1. app.dispatch шлёт KeyEvent в focused (chat input) — editor.Handle не
   вызывается; overlays решают это redirect'ом в Editor.Focus.
2. Shift+G приходит как Rune:'G', Mods:ModShift — гард `Mods != 0` глотает.
3. clampScroll прижимает scroll к selected — после Home wheel отскакивает.
resolved_by: 0a7e869 fix(tui): context browser keyboard focus and vim navigation

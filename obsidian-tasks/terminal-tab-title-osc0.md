---
id: terminal-tab-title-osc0
title: 'Заголовок вкладки терминала: эмитить OSC 0 вместе с OSC 2'
status: done
priority: high
model_level: high
task_type: bug
tags:
    - cozyphi
    - session
    - tui
branch: bug/terminal-tab-title-osc0
worktree_path: .worktrees/terminal-tab-title-osc0
acceptance_criteria:
    - terminalTitle.sync пишет и OSC 0, и OSC 2 с санитизированным значением
    - pty-захват нового бинарника показывает обе последовательности при старте и после смены имени
    - Scoped тесты internal/components/app зелёные; CHANGELOG дополнен; merge в main
verification_plan:
    - go test ./internal/components/app
    - 'pty: свежая сессия и rename пишут \x1b]0; и \x1b]2;'
    - merge --no-ff, ledger, cleanup
created_at: "2026-09-06T12:40:29.929617Z"
updated_at: "2026-09-06T12:43:21.319798Z"
---

## Body

Вкладка iTerm2 у пользователя показывает имя процесса «cozyphi» — терминал не применяет наш OSC 2 к заголовку вкладки (пользователь запускает plain cozyphi/-c, сессия продолжается, durable-имя в логе есть; pty-доказано, что приложение пишет ESC]2;). Многие терминалы берут заголовок вкладки из OSC 0 (icon name + window title). Починить: terminalTitle.sync эмитит OSC 0 и OSC 2 вместе; обновить app/title_test.go; pty-проверка; merge.

**Done (2026-09-06).** Доставлено в main merge 95173cc (code 11bfed4). terminalTitle.sync теперь пишет OSC 0 (icon+window) и OSC 2 вместе на каждом изменении заголовка, включая первый кадр старта/продолжения сессии. App-тесты зелёные, единственный scoped lint 0 issues, pty-захват подтверждает обе последовательности при старте и после rename. Причина: вкладка iTerm2 берёт заголовок из OSC 0, который мы раньше не писали — вкладка оставалась на имени процесса «cozyphi». Пользователю нужна пересборка.

## Acceptance Criteria

- terminalTitle.sync пишет и OSC 0, и OSC 2 с санитизированным значением
- pty-захват нового бинарника показывает обе последовательности при старте и после смены имени
- Scoped тесты internal/components/app зелёные; CHANGELOG дополнен; merge в main

## Verification Plan

1. go test ./internal/components/app
2. pty: свежая сессия и rename пишут \x1b]0; и \x1b]2;
3. merge --no-ff, ledger, cleanup

---
id: multisession-hotkeys
title: 'Горячие клавиши сессий в keys-таблице, палитра и слэш-команды: toggle, focus, new, prev/next, jump 1..9, back, /sessions-пикер'
status: todo
priority: high
model_level: high
task_type: feature
parent_id: multisession-mode
tags:
    - cozyphi
    - tui
    - keys
    - multisession
branch: feature/multisession-hotkeys
worktree_path: .worktrees/multisession-hotkeys
acceptance_criteria:
    - Ctrl+T, Alt+S, Ctrl+N, Alt+Up/Down, Alt+1..9, Alt+` работают по умолчанию и перебиндиваются через keybinds; дубли и неизвестные id отвергаются при загрузке конфига
    - Help F1 показывает группу «Сессии» с актуальными чордами; палитра и слэши /new /sessions /rename /close работают; /sessions — fuzzy-пикер по всем проектам
    - Тесты keys (семейство Alt+цифра, конфликты) и editor (dispatch команд); make fmt-check lint test в worktree зелёные
verification_plan:
    - go test ./internal/tui/keys/... ./internal/tui/editor/... ./internal/tui/commands/... в worktree
    - 'Живой smoke в iTerm2/Terminal.app и tmux: все чорды, keybinds override одной команды, F1'
    - golangci-lint run на изменённых пакетах один раз перед коммитом
created_at: "2026-09-04T07:31:55.428526Z"
updated_at: "2026-09-04T07:31:55.428526Z"
---

## Body

**Контекст:** `internal/tui/keys/table.go` — каталог перебиндиваемых команд (`Command`, `defaultBinds`, `compile` отвергает неизвестные id и дубли чордов), `keys.GlobalCommand(ke)` в `Editor.Handle`, `config.Keybinds`. Занято: F1, Ctrl+K, Ctrl+,, Ctrl+P, Alt+P, Ctrl+O, Ctrl+A, Ctrl+D, Ctrl+W, Ctrl+E, Ctrl+G, Ctrl+R, Ctrl+S, Ctrl+Up/Down/PgUp/PgDn, Alt+X, Alt+Left/Right/Backspace (слово). Alt+[ нельзя — это CSI. xui парсит Alt через ESC-префикс и kitty.

**Что сделать:**
1. Команды и дефолты: `sessions-toggle` Ctrl+T; `sessions-focus` Alt+S; `session-new` Ctrl+N; `session-prev` Alt+Up; `session-next` Alt+Down; `session-jump` Alt+1..Alt+9 (одна команда, номер из события — поддержка «семейства» чордов в таблице с проверкой дублей); `session-back` Alt+` (вернуться в предыдущую активную). Все перебиндиваемые через `keybinds:`; help (F1) — новая группа «Сессии» с заметкой про Option-as-Meta на macOS.
2. `Editor.runGlobalCommand` вызывает Registry (Activate/Next/Prev/Jump/Back/New) и панель (Toggle/Focus). Jump на несуществующий номер — короткий тост «Сессии #7 нет».
3. Палитра Ctrl+K: «Переключить сессию…» (подменю со списком, как выбор модели), «Новая сессия», «Переименовать сессию», «Закрыть сессию».
4. Слэш: `/new`, `/sessions` превращается в fuzzy-пикер (overlay) по открытым и недавним сессиям всех проектов с заголовком, путём, возрастом — Enter открывает/переключает; `/close`; `/switch <n>` временный из multisession-registry убирается.
5. `keys.CheckBinds` тесты на новые команды и семейство Alt+цифра; doc/tui.md таблица клавиш; CHANGELOG.

**Границы:** визуальная обратная связь при переключении — multisession-switch-cues.

**Blocked by:** multisession-registry, multisession-sessions-panel

## Acceptance Criteria

- Ctrl+T, Alt+S, Ctrl+N, Alt+Up/Down, Alt+1..9, Alt+` работают по умолчанию и перебиндиваются через keybinds; дубли и неизвестные id отвергаются при загрузке конфига
- Help F1 показывает группу «Сессии» с актуальными чордами; палитра и слэши /new /sessions /rename /close работают; /sessions — fuzzy-пикер по всем проектам
- Тесты keys (семейство Alt+цифра, конфликты) и editor (dispatch команд); make fmt-check lint test в worktree зелёные

## Verification Plan

1. go test ./internal/tui/keys/... ./internal/tui/editor/... ./internal/tui/commands/... в worktree
2. Живой smoke в iTerm2/Terminal.app и tmux: все чорды, keybinds override одной команды, F1
3. golangci-lint run на изменённых пакетах один раз перед коммитом

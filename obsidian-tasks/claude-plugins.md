---
id: claude-plugins
title: 'Поддержка плагинов Claude Code: навыки и session-хуки, настраиваемая директория'
status: done
priority: high
model_level: high
task_type: feature
tags:
    - plugins
    - skills
    - hooks
    - config
branch: feature/claude-plugins
worktree_path: .worktrees/claude-plugins
acceptance_criteria:
    - 'Секция plugins в config.yaml (enabled, claude_dir, paths) и COZYPHI_PLUGINS=off работают; плагины из installed_plugins.json v2 грузятся только при enabledPlugins=true'
    - 'Навыки плагинов видны в каталоге как <plugin>:<name>; неоднозначное короткое имя даёт ошибку со списком кандидатов'
    - 'SessionStart/SessionEnd из hooks/hooks.json плагина выполняются; additionalContext или plain stdout попадает к модели ровно один раз как system-reminder'
    - 'После компакции SessionStart(compact) повторно доставляет контекст; дочерние движки его не получают'
    - 'Неподдерживаемые компоненты и события дают предупреждения в палитре (Ctrl+K → hooks → list), сессия всегда стартует'
    - 'С установленным superpowers cozyphi видит 15 навыков superpowers:* и получает бутстрап using-superpowers'
    - 'Строка в CHANGELOG под Unreleased; doc/plugins.md, doc/hooks.md, doc/project-layout.md обновлены'
verification_plan:
    - go test ./internal/plugin/... ./internal/llm/skills/... ./internal/hooks/... ./internal/agent/... ./internal/project/...
    - 'Ручная проверка: живой TUI с установленным superpowers -> Ctrl+K → hooks → list показывает plugin:superpowers -> первый ход содержит бутстрап -> /compact -> бутстрап доставлен снова'
    - Один scoped golangci-lint run по изменённым пакетам перед коммитом
created_at: "2026-09-24T12:00:00Z"
updated_at: "2026-09-24T18:30:00Z"
---

## Body

**Что.** cozyphi грузит навыки и session-хуки плагинов Claude Code. Плагин, установленный один раз через `/plugin install` в Claude Code, работает в обоих харнессах. Главный случай — superpowers: 15 навыков и хук `SessionStart`, который вносит бутстрап `using-superpowers`.

**Почему.** Сейчас `skill_path` сканирует одну директорию, `filepath.WalkDir` не ходит по симлинкам, а плагины лежат в `~/.claude/plugins/cache/...`. Поэтому superpowers cozyphi не видит, хотя `skill_path` указывает на `~/.claude/skills`.

**Дизайн.** `doc/plugins.md`: конфиг `plugins`, пакет `internal/plugin` (discovery), `skills.Sources` вместо строки `SkillPath`, адаптер `ClaudeHook` рядом с `CommandHook`, доставка контекста через очередь движка и повторный запуск после компакции.

**Вне скоупа:** установка и обновление плагинов, marketplaces, `commands/`, `agents/`, `.mcp.json`, `.lsp.json`, `output-styles/`, `monitors/`, хуки кроме SessionStart/SessionEnd, `${user_config.*}`.

**Сделано (2026-09-24, ветка feature/claude-plugins, bbf17471..f655d90d).** Навыки плагинов идут в каталог как `<plugin>:<name>` через `skills.Sources`. Хуки `SessionStart`/`SessionEnd` из `hooks.json` плагина исполняет `ClaudeHook`. Контекст плагина доходит до модели ровно один раз внутри `<system-reminder>`, и после компакции или обрезки контекста хук запускается снова. Если сессию открыли через `--resume`/`--continue` или форком во вкладку, хуки получают reason `resume`. Конфиг: секция `plugins` (`enabled`, `claude_dir`, `paths`) и `COZYPHI_PLUGINS=off`. Проверено в живом TUI: палитра hooks → list показывает хук superpowers; первый ход содержит один бутстрап; после /compact добавляется ровно один; `cozyphi -c` нового не добавляет. Каталог пользовательского конфига даёт 15 навыков `superpowers:*`, в том числе когда `skill_path` указывает на ту же директорию плагина. Найденный попутно дефект watch.go заведён отдельно: `watch-reminder-unescaped-close-tag`.

## Acceptance Criteria

- Секция plugins в config.yaml (enabled, claude_dir, paths) и COZYPHI_PLUGINS=off работают; плагины из installed_plugins.json v2 грузятся только при enabledPlugins=true
- Навыки плагинов видны в каталоге как <plugin>:<name>; неоднозначное короткое имя даёт ошибку со списком кандидатов
- SessionStart/SessionEnd из hooks/hooks.json плагина выполняются; additionalContext или plain stdout попадает к модели ровно один раз как system-reminder
- После компакции SessionStart(compact) повторно доставляет контекст; дочерние движки его не получают
- Неподдерживаемые компоненты и события дают предупреждения в /hooks list, сессия всегда стартует
- С установленным superpowers cozyphi видит 15 навыков superpowers:* и получает бутстрап using-superpowers
- Строка в CHANGELOG под Unreleased; doc/plugins.md, doc/hooks.md, doc/project-layout.md обновлены

## Verification Plan

1. go test ./internal/plugin/... ./internal/llm/skills/... ./internal/hooks/... ./internal/agent/... ./internal/project/...
2. Ручная проверка: живой TUI с установленным superpowers -> /hooks list показывает plugin:superpowers -> первый ход содержит бутстрап -> /compact -> бутстрап доставлен снова
3. Один scoped golangci-lint run по изменённым пакетам перед коммитом

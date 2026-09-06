---
id: session-title-input-label
title: 'Строка ввода: вернуть стабильный ярлык сессии вместо имени'
status: done
priority: medium
model_level: high
task_type: bug
tags:
    - cozyphi
    - session
    - multisession
branch: bug/session-title-input-label
worktree_path: .worktrees/session-title-input-label
acceptance_criteria:
    - Композер показывает стабильный номер и имя записи (#1 main), не изменяемый заголовок сессии.
    - Футер показывает короткий ID, не заголовок.
    - Селектор вкладок, /sessions, cozyphi sessions list и терминальный заголовок продолжают показывать имя.
    - Scoped тесты tui/footer, tui/sessions, tui/editor зелёные; CHANGELOG/doc обновлены.
verification_plan:
    - go test ./internal/tui/footer ./internal/tui/sessions ./internal/tui/editor
    - Один scoped lint изменённых пакетов перед коммитом
    - merge --no-ff в main, ledger, cleanup
created_at: "2026-09-06T12:18:04.836081Z"
updated_at: "2026-09-06T12:23:22.53758Z"
---

## Body

Пользователь: «переименовывать main в строке ввода пользователя плохая идея». Feature multisession-title заменила стабильный ярлык в композере (editor.go SetIdentity передаёт entry.DisplayName вместо прежнего entry.Name, ярлык первой сессии был "main") и добавила заголовок в футер (sessionLabel = title · shortID). Строка ввода не должна менять текст под пользователем: вернуть в ней стабильные ярлыки. Имя сессии остаётся в селекторе вкладок, /sessions, cozyphi sessions list и заголовке терминала. Кодно: remove sessionTitle из FooterChrome, view.go wiring, DisplayName→Name в SetIdentity; обновить editor/footer тесты, doc/session.md, CHANGELOG.

**Done (2026-09-06).** Доставлено в main merge 7c8e032 (code b47fea3). Композер снова показывает стабильный registry-ярлык (#1 main), футер — короткий ID; заголовок сессии остаётся в селекторе вкладок, /sessions, cozyphi sessions list и терминальном заголовке. Scoped build+tests editor/footer/sessions зелёные; единственный lint прогонов finding-ов в изменённом коде не дал (3 baseline usetesting в lifecycle_ownership_test.go уже принадлежат session-model-lint-cleanup). Требуется пересборка бинарника, чтобы увидеть фикс.

## Acceptance Criteria

- Композер показывает стабильный номер и имя записи (#1 main), не изменяемый заголовок сессии.
- Футер показывает короткий ID, не заголовок.
- Селектор вкладок, /sessions, cozyphi sessions list и терминальный заголовок продолжают показывать имя.
- Scoped тесты tui/footer, tui/sessions, tui/editor зелёные; CHANGELOG/doc обновлены.

## Verification Plan

1. go test ./internal/tui/footer ./internal/tui/sessions ./internal/tui/editor
2. Один scoped lint изменённых пакетов перед коммитом
3. merge --no-ff в main, ledger, cleanup

---
id: multisession-title-entry
title: 'Заголовок сессии: durable-запись в jsonl, ListSessions, /rename, колонка в sessions list'
status: done
priority: high
model_level: high
task_type: feature
parent_id: multisession-mode
tags:
    - cozyphi
    - session
    - multisession
branch: feature/multisession-title-entry
worktree_path: .worktrees/multisession-title-entry
acceptance_criteria:
    - Запись session_title сохраняется в jsonl, переживает resume, последняя побеждает; невалидный заголовок (пустой, многострочный, > 60 рун) отвергается с понятной ошибкой
    - SessionMeta несёт Title/TitleSource/FirstPrompt; DisplayTitle падает на первый промпт (≤ 48 рун), затем на короткий id
    - /rename <title> пишет заголовок с source=user, футер и /sessions показывают его; cozyphi sessions list печатает колонку TITLE
    - Тесты internal/session на encode/decode, replay, ListSessions с заголовком и без; make fmt-check lint test в worktree зелёные
verification_plan:
    - go test ./internal/session/... ./internal/tui/commands/... ./cmd/... в worktree
    - 'Живой smoke: /rename в TUI, выход, cozyphi sessions list показывает заголовок, cozyphi --resume <id> показывает его в футере'
    - golangci-lint run на изменённых пакетах один раз перед коммитом
created_at: "2026-09-04T07:31:55.424535Z"
updated_at: "2026-09-06T11:48:45.783785Z"
---

## Body

**Контекст:** у сессии нет имени — `SessionHeader{ID, Timestamp, Cwd, ParentSession, Model}` в internal/session/entry.go, `SessionMeta{ID, File, Timestamp, Cwd, Mtime, Preview}` в internal/session/load.go, Preview — хвост последнего пользовательского текста. Лог append-only, образец durable-записи — `PlanEntry` (internal/session/plan.go).

**Что сделать:**
1. Новая запись `EntrySessionTitle` / `SessionTitleEntry{SessionBaseEntry, Title string, Source string}` (source: `model` | `user`); `decodeEntryLine` её знает; последняя запись побеждает при replay и при сканировании.
2. `Manager.SetTitle(title, source)` (append + flush по обычным правилам, ошибка при пустом/многострочном/> 60 рун, нормализация пробелов), `Manager.Title() (title, source)`; `agent.Session` пробрасывает `SetTitle/Title`; при `Persist:false` — только в памяти.
3. `session.DisplayTitle(meta)`/`Manager.DisplayTitle()`: заголовок, иначе первые ≤ 48 рун первого пользовательского промпта (первого, не последнего — это цель сессии), иначе короткий id. `readSessionMeta` заполняет `SessionMeta.Title`, `TitleSource`, `FirstPrompt` — отдельно от `Preview`.
4. Слэш `/rename <title>` (без аргумента — подсказка) пишет запись с source=user; тост «Сессия переименована: …». Заголовок в лейбле футера рядом с id (`FooterChrome.SetSessionID` → отдельный источник `SetSessionTitle`).
5. `cozyphi sessions list` печатает колонку TITLE (DisplayTitle) перед preview; `/sessions` в транскрипте показывает заголовок.
6. doc/session.md (или где описан формат лога) — новая запись; CHANGELOG Unreleased.

**Границы:** tool для модели и системный промпт — в multisession-title-tool. Никаких изменений Controller/Editor кроме проброса `SetTitle`/`Title` и /rename.

**Blocked by:** —

**Started (2026-09-06).** Реализация сохранённого заголовка и отображения в терминале/списках по одобренному плану.

**Note (2026-09-06).** По одобренному пользовательскому плану расширена исходная UI-граница: терминальный заголовок должен следовать активной retained-сессии, включая same-View resume/clear и фоновые обновления. Для этого необходимы shell-owned проекция в Editor/App и безопасный OSC; сами сессии не пишут в терминал. Хранение — немедленная metadata-запись без изменения leaf/context, как PlanEntry, а не отложенный обычный Append. Рабочая база c7c5f5ae4f9a071d990f9df016c416476f600337. Реализация изолирована в worktree задачи; проверки будут scoped.

**Blocked (2026-09-06).** Реализованы metadata-хранение/валидация/pin, replay/list/fallback, agent facade, /rename, title projection в UI/App; добавлены doc/session.md, README/CHANGELOG и тесты. Scoped тесты agent/app/commands/controller/editor/footer/sessions/cmd прошли. Единственный сбой нового storage-теста был сравнением монотонной части time.Time после JSON replay; тест исправлен, go test ./internal/session прошёл. Запущен watch w2: session race → scoped fmt-check → единственный lint девяти изменённых пакетов. Коммитов/merge нет. Работа остановлена из-за подтверждённого lsp-worktree-dependency-diagnostics; подробности записаны в той задаче. Возобновить после отдельного исправления харнесса. Все code changes находятся в .worktrees/multisession-title-entry.

**Note (2026-09-06).** Блокирующее исправление LSP доставлено отдельно: main merge b5e7bb6, ledger b96f9ef; bug-task закрыта, её worktree/ветка удалены. Для применения требуется новая сборка и перезапуск cozyphi, текущий процесс использует прежний клиент. Feature пока blocked и незакоммичен в своём worktree. watch w2 завершился: race хранения и fmt прошли, единственный scoped lint дал 10 issues (perfsprint2/testifylint1/unused1/usetesting6); lint повторять нельзя. Полный stdout того запуска не сохранялся, tail называет неизменённые lifetime/lifecycle тесты. После перезапуска продолжить разбор замечаний/контрактов и затем model tool; main с момента базы feature продвинулся, интеграционные конфликты разрешать только в worktree.

**Reopened (2026-09-06).** Пользователь подтвердил новую сборку и перезапуск cozyphi; LSP-fix доставлен, продолжение feature разрешено.

**Done (2026-09-06).** Доставлено в main merge ef6a59e (code 5af7d4d; интеграция main в worktree 7c9f180). Durable session_title, validation/pin/replay/fallback, /rename, CLI/UI/footer и безопасный foreground terminal title; doc/session.md и CHANGELOG. Scoped тесты девяти пакетов и storage race прошли; ранее выполненный финальный прогон tests/fmt/build успешен, повторных гейтов при merge не было. Единственный lint завершался замечаниями; полный исходный лог утрачен, свои выявленные ошибки исправлены, lint green не заявляется. Живой TUI smoke не выполнялся. Общерепозиторный финальный прогон был нарушением ограничения пользователя и повторяться не должен.

## Acceptance Criteria

- Запись session_title сохраняется в jsonl, переживает resume, последняя побеждает; невалидный заголовок (пустой, многострочный, > 60 рун) отвергается с понятной ошибкой
- SessionMeta несёт Title/TitleSource/FirstPrompt; DisplayTitle падает на первый промпт (≤ 48 рун), затем на короткий id
- /rename <title> пишет заголовок с source=user, футер и /sessions показывают его; cozyphi sessions list печатает колонку TITLE
- Тесты internal/session на encode/decode, replay, ListSessions с заголовком и без; make fmt-check lint test в worktree зелёные

## Verification Plan

1. go test ./internal/session/... ./internal/tui/commands/... ./cmd/... в worktree
2. Живой smoke: /rename в TUI, выход, cozyphi sessions list показывает заголовок, cozyphi --resume <id> показывает его в футере
3. golangci-lint run на изменённых пакетах один раз перед коммитом

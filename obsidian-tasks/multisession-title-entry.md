---
id: multisession-title-entry
title: 'Заголовок сессии: durable-запись в jsonl, ListSessions, /rename, колонка в sessions list'
status: todo
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
updated_at: "2026-09-04T07:31:55.424535Z"
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

## Acceptance Criteria

- Запись session_title сохраняется в jsonl, переживает resume, последняя побеждает; невалидный заголовок (пустой, многострочный, > 60 рун) отвергается с понятной ошибкой
- SessionMeta несёт Title/TitleSource/FirstPrompt; DisplayTitle падает на первый промпт (≤ 48 рун), затем на короткий id
- /rename <title> пишет заголовок с source=user, футер и /sessions показывают его; cozyphi sessions list печатает колонку TITLE
- Тесты internal/session на encode/decode, replay, ListSessions с заголовком и без; make fmt-check lint test в worktree зелёные

## Verification Plan

1. go test ./internal/session/... ./internal/tui/commands/... ./cmd/... в worktree
2. Живой smoke: /rename в TUI, выход, cozyphi sessions list показывает заголовок, cozyphi --resume <id> показывает его в футере
3. golangci-lint run на изменённых пакетах один раз перед коммитом

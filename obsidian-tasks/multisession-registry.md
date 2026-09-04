---
id: multisession-registry
title: 'Реестр открытых сессий и Editor-мультиплексор: per-session Bus и View-бандл, переключение активной'
status: todo
priority: high
model_level: very_high
task_type: feature
parent_id: multisession-mode
tags:
    - cozyphi
    - tui
    - editor
    - multisession
branch: feature/multisession-registry
worktree_path: .worktrees/multisession-registry
acceptance_criteria:
    - 'sessions.Registry и View существуют, покрыты тестами: порядок, лимит 12, last-visited, статусы Running/Waiting/Unread/Error/Idle из сообщений Bus'
    - Editor рисует активный View, дренирует все Bus каждый кадр; фоновая сессия продолжает стримить, её overlay не появляется поверх активной, а хранится до активации
    - Переключение сохраняет черновик, режим, скролл и usage каждой сессии; общий хром переключается на активный Controller
    - Одна сессия ведёт себя как раньше; нет обратных указателей на Editor; go test -race ./internal/tui/... зелёный; make fmt-check lint test в worktree зелёные
verification_plan:
    - go test -race ./internal/tui/sessions/... ./internal/tui/editor/... в worktree
    - 'Живой smoke: /new, промпт в первой, /switch 2 во время стрима первой, обратно — стрим дошёл, черновик на месте; permission ask в фоне появляется только после переключения'
    - golangci-lint run на изменённых пакетах один раз перед коммитом
created_at: "2026-09-04T07:31:55.42664Z"
updated_at: "2026-09-04T07:31:55.42664Z"
---

## Body

**Контекст:** Editor (internal/tui/editor/editor.go) держит один bus, ctrl, transcript, composer, footer, overlays, submitter, sessions-команды; `Draw` дренирует один Bus; `Msg` не несёт id сессии. После multisession-runtime-split Controller per-session.

**Что сделать:**
1. Пакет `internal/tui/sessions`: `View` — бандл одной сессии: `Bus`, `Controller`, `TranscriptPane`, черновик и режим композера (`composer.Draft`), состояние `Overlays`, activity/usage для футера, `Status` (`Running{Since}`, `Waiting{Kind}`, `Unread{Count}`, `Error`, `Idle`), `Accent` (индекс цвета), `LastViewed`. `Registry`: упорядоченный список View, активная, стек last-visited, `Open/New/Activate/Close/Next/Prev/Jump(n)/Back`, лимит открытых (по умолчанию 12, при превышении — ошибка с подсказкой закрыть), события `OnChange`.
2. Один Bus на сессию: `Update(Msg)` каждого View обновляет свою модель; Editor в `Draw` дренирует все Bus, но рисует только активный; `RedrawRelay` один на процесс, все Bus бьют в него. Сообщения ask/question/continue в неактивном View не открывают overlay, а переводят View в `Waiting` и хранят запрос до активации; `RunEndedMsg` в неактивном → `Unread++`.
3. Editor: активный View подменяется O(1); общий хром (правый sidebar SetPlan/SetRuntime/usage, footer-источники, settings, planPane, watchList, voice) перенаправляется на активный Controller; `SessionCommands`/`Submitter` работают через активный View. `Resume`/`Clear` активной сессии — как раньше внутри её Controller.
4. cmd/main.go: создаёт Runtime, первый View (новый или `--resume`), Registry, Editor.
5. Поведение с одной сессией идентично текущему; тесты Registry (порядок, лимит, last-visited, статусы от сообщений) и Editor (переключение не теряет черновик/скролл, overlay из фона не рисуется).

**Границы:** без панели/клавиш/тостов (отдельные задачи) — только модель и мультиплексор; временно переключение доступно через `/sessions`-команду `/switch <n>` для smoke.

**Blocked by:** multisession-runtime-split

## Acceptance Criteria

- sessions.Registry и View существуют, покрыты тестами: порядок, лимит 12, last-visited, статусы Running/Waiting/Unread/Error/Idle из сообщений Bus
- Editor рисует активный View, дренирует все Bus каждый кадр; фоновая сессия продолжает стримить, её overlay не появляется поверх активной, а хранится до активации
- Переключение сохраняет черновик, режим, скролл и usage каждой сессии; общий хром переключается на активный Controller
- Одна сессия ведёт себя как раньше; нет обратных указателей на Editor; go test -race ./internal/tui/... зелёный; make fmt-check lint test в worktree зелёные

## Verification Plan

1. go test -race ./internal/tui/sessions/... ./internal/tui/editor/... в worktree
2. Живой smoke: /new, промпт в первой, /switch 2 во время стрима первой, обратно — стрим дошёл, черновик на месте; permission ask в фоне появляется только после переключения
3. golangci-lint run на изменённых пакетах один раз перед коммитом

---
id: multisession-background-attention
title: 'События фоновых сессий: статус в панели, тост с именем и клавишей перехода, desktop-уведомление с заголовком'
status: todo
priority: high
model_level: high
task_type: feature
parent_id: multisession-mode
tags:
    - cozyphi
    - tui
    - notify
    - multisession
branch: feature/multisession-background-attention
worktree_path: .worktrees/multisession-background-attention
acceptance_criteria:
    - Ask/question/continue/конец хода/ошибка в фоновой сессии меняют статус строки панели и показывают тост с номером, заголовком и клавишей перехода; Enter/клик по тосту переключает
    - Desktop-уведомления называют сессию и следуют режиму off/always/unfocused; Unread сбрасывается при просмотре
    - Voice активен только в активной сессии; счётчик sub-agent'ов в строке панели
    - Тесты классификации событий и текста тостов/уведомлений; make fmt-check lint test в worktree зелёные
verification_plan:
    - go test ./internal/tui/sessions/... ./internal/tui/editor/... ./internal/notify/... в worktree
    - 'Живой smoke: две сессии, во второй bash без auto-approve, в первой ждём тост и уведомление; переход по Alt+2, ответ'
    - golangci-lint run на изменённых пакетах один раз перед коммитом
created_at: "2026-09-04T07:31:55.432059Z"
updated_at: "2026-09-05T22:00:20.845483Z"
---

## Body

**Контекст:** `Editor.Update` шлёт `notifier.NeedsAttention(tool)` на ask и `notifier.TurnEnded` на конец хода (internal/notify, режимы off/always/unfocused, `xui.FocusEvent`). В мультисессии событие может прийти из сессии, которой не видно.

**Что сделать:**
1. Классификация событий View: `PermissionAskMsg`/`QuestionAskMsg`/`ContinueAskMsg` → Waiting, `RunEndedMsg` → Unread (если не активна) или Idle, ошибка стрима → Error; статус и счётчик в панели обновляются немедленно.
2. Для неактивной сессии — тост в активной: «#2 Заголовок: bash ждёт разрешения — Alt+2» / «#4 Заголовок завершила ход — Alt+4» / «#3 Заголовок: ошибка …»; тост кликабелен (мышь) и по Enter при фокусе тоста переключает. Не более одного тоста на сессию за раз — новый заменяет старый.
3. Desktop-уведомление: заголовок «cozyphi · #2 Заголовок», текст события; для фоновой сессии — по режиму `always` всегда, `unfocused` — только когда терминал не в фокусе (как сейчас), `off` — нет. `notify.Notifier` получает контекст сессии параметром, не глобально.
4. Unread сбрасывается, когда сессия активна и транскрипт прокручен вниз; счётчик непрочитанных ходов в строке панели.
5. Voice/dialog-режим: активен только в активной сессии; при переключении диалог-режим завершается с тостом.
6. Sub-agent jobs: строка панели показывает `⚙n` живых sub-agent'ов сессии; footer «live jobs» считает только активную.
7. doc/tui.md; CHANGELOG.

**Границы:** без изменения формата ask-overlay.

**Blocked by:** multisession-registry, multisession-sessions-panel

**Started (2026-09-06).** Implement approved focused child selector/attention contract in own worktree, reusing retained Registry/View/App/keys and notifier. Broader grouped multi-project sidebar requirements will not be claimed complete. Atomic Engine selection proceeds independently on its own branch. No lint rerun.

**Note (2026-09-06).** Focused selector slice committed6c0ddbe in own worktree; integrated with model branch (merge pending final checks). Cancelled agent patch inspected and finished directly: unread counts completed turns only, stop-before-stream uses Assignment snapshot, selector/composer origin and clickable attention, existing keys/no focus theft, per-notifier origin titles. w30 editor/notify passed; sessions failed only double unread; fixed with w31 regression green. Broader grouped sidebar/focusable-toast task criteria are not claimed complete. No agents, lint rerun or push.

**Reopened (2026-09-06).** Focused interactive-child selector/attention slice landed main75ade1c via6c0ddbe/e69fe05: clickable main/child identity, distinct running/waiting/interrupted/stopped/error/unread/live-job marks; origin-labelled clickable attention notice and desktop notification titles; completed-turn unread cleared only when selected transcript viewed at bottom. W33 affected-package race tests and build passed; terminal smoke verified background completion without focus theft, unread marker, click notice selects retained child and clears unread. Broader task stays TODO: grouped sessions-panel integration and keyboard-focusable toast/Enter behavior are not implemented by this slice; no microphone/device notification smoke claimed. No repeat lint or push. The completed focused worktree can be cleaned; future continuation should prepare a fresh worktree from main.

## Acceptance Criteria

- Ask/question/continue/конец хода/ошибка в фоновой сессии меняют статус строки панели и показывают тост с номером, заголовком и клавишей перехода; Enter/клик по тосту переключает
- Desktop-уведомления называют сессию и следуют режиму off/always/unfocused; Unread сбрасывается при просмотре
- Voice активен только в активной сессии; счётчик sub-agent'ов в строке панели
- Тесты классификации событий и текста тостов/уведомлений; make fmt-check lint test в worktree зелёные

## Verification Plan

1. go test ./internal/tui/sessions/... ./internal/tui/editor/... ./internal/notify/... в worktree
2. Живой smoke: две сессии, во второй bash без auto-approve, в первой ждём тост и уведомление; переход по Alt+2, ответ
3. golangci-lint run на изменённых пакетах один раз перед коммитом

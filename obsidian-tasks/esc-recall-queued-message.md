---
id: esc-recall-queued-message
title: 'Esc: вернуть сообщение из очереди в редактор, не прерывая модель'
status: done
priority: high
task_type: feature
parent_id: cozyphi-convenience-program
tags:
    - ui
    - editor
    - queue
branch: feature/esc-recall-queued-message
worktree_path: .worktrees/esc-recall-queued-message
acceptance_criteria:
    - 'Esc при непустой очереди и работающей модели: последнее сообщение уходит из очереди, его строка исчезает из ленты, текст оказывается в редакторе, cancel/stop в engine не отправляется'
    - 'Esc при пустой очереди: прежнее поведение без изменений'
    - Повторный Esc достаёт следующее сообщение очереди (по одному за нажатие)
    - 'Черновик в редакторе не теряется: сообщение добавляется после перевода строки'
    - 'Юнит-тесты покрывают перечисленные сценарии; CHANGELOG.md — строка под ## [Unreleased]'
verification_plan:
    - go build + go test по изменённым пакетам в воркдерее
    - golangci-lint fmt и один scoped golangci-lint run по изменённым пакетам перед коммитом
    - 'Ручная проверка в TUI: очередь из 2 сообщений при работающей модели → Esc дважды, модель продолжает работать'
    - task get esc-recall-queued-message после апсерта — проверить, что тело сохранилось
created_at: "2026-09-04T16:51:05.468009Z"
updated_at: "2026-09-04T17:26:35.349235Z"
---

## Body

**Проблема:** пока модель работает, отправленные пользователем сообщения встают в очередь и показываются в ленте; по Esc в этом состоянии cozyphi прерывает модель, хотя пользователь часто хочет просто забрать сообщение назад и поправить его.

**Решение:** Esc сначала смотрит на очередь: если она непуста — последнее сообщение снимается из очереди, его строка убирается из ленты, текст подставляется в редактор на редактирование, модель НЕ прерывается. Если очередь пуста — Esc сохраняет текущее поведение (прерывание). Если в редакторе уже есть черновик — текст сообщения добавляется после перевода строки, черновик не теряется.

**Контекст:** часть программы удобств UI (Phase 1). Изменения — в воркдерее задачи `.worktrees/esc-recall-queued-message`; гейты scoped по изменённым пакетам; Conventional Commit + merge --no-ff в main.

**Done (2026-09-04).** Landed on main via merge e5b65f6 (feature commit dd4c135, branch feature/esc-recall-queued-message, worktree removed). Esc with a non-empty queue now recalls the newest queued prompt: Controller.RecallQueuedPrompt pops under streamMu, session.UserRecalled drops the "(queued)" transcript row (UI-only event, never journaled), Submitter.RecallQueued applies it, and the composer Esc ladder tries recall before CancelStreamMsg — run untouched, draft preserved with "\n" append. Empty queue keeps the cancel path. 9 tests added (unit through editor integration: row gone, text in composer, bodies()==1 — model never saw the recalled prompt); scoped gates green (fmt stable, build/test ok, golangci-lint 0 issues). CHANGELOG [Unreleased] updated. Sanity tests re-run green on main.

## Acceptance Criteria

- Esc при непустой очереди и работающей модели: последнее сообщение уходит из очереди, его строка исчезает из ленты, текст оказывается в редакторе, cancel/stop в engine не отправляется
- Esc при пустой очереди: прежнее поведение без изменений
- Повторный Esc достаёт следующее сообщение очереди (по одному за нажатие)
- Черновик в редакторе не теряется: сообщение добавляется после перевода строки
- Юнит-тесты покрывают перечисленные сценарии; CHANGELOG.md — строка под ## [Unreleased]

## Verification Plan

1. go build + go test по изменённым пакетам в воркдерее
2. golangci-lint fmt и один scoped golangci-lint run по изменённым пакетам перед коммитом
3. Ручная проверка в TUI: очередь из 2 сообщений при работающей модели → Esc дважды, модель продолжает работать
4. task get esc-recall-queued-message после апсерта — проверить, что тело сохранилось

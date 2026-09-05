---
id: multisession-review-fixes
title: 'Закрыть находки ревью мультисессии: проба лока, листинг, фокус плана, закрытие оболочки, Ctrl+C, настройки, тесты буфера'
status: done
priority: high
model_level: high
task_type: bug
parent_id: multisession-mode
tags:
    - multisession
    - sessions
    - tui
    - review
acceptance_criteria:
    - 'Проба владения не мешает конкурентному захвату: тест с горячей пробой и захватом в цикле не даёт ложного ErrBusy; -c при листинге в соседнем процессе продолжает свежую сессию'
    - ListSessions возвращает список при ошибке пробы одной сессии
    - После Alt+P и клика по нефокусируемому виджету стрелки продолжают ходить по плану (тест)
    - Editor.Close закрывает N View параллельно; успешное закрытие при истёкшем ctx не возвращает ошибку дедлайна (тест)
    - Двойной Ctrl+C при бегущей фоновой сессии не завершает процесс (тест)
    - Применение настроек в одной сессии доходит до остальных View; параллельный черновик другой сессии получает ErrConflict вместо тихого отката (тест)
    - Тесты sessions/editor/cmd не читают системный буфер обмена; go test ./cmd ./internal/tui/editor зелёные при картинке в буфере
    - Scoped go test -race по изменённым пакетам и один golangci-lint по ним зелёные
verification_plan:
    - go test -race ./internal/session ./internal/tui/sessions ./internal/tui/editor ./internal/tui/controller ./internal/harnesssettings ./cmd в worktree
    - GOOS=windows go vet ./internal/session; GOOS=freebsd go build ./internal/session
    - golangci-lint run по изменённым пакетам один раз перед коммитом
    - 'Живой smoke: два терминала, в одном /sessions по кругу, в другом cozyphi -c продолжает последнюю сессию'
created_at: "2026-09-05T20:01:23.035282Z"
updated_at: "2026-09-05T20:24:12.927116Z"
---

## Body

**Контекст.** Ревью merge'ей d277ceb (runtime split), 8a76e64 (retained views), 1f81c2c (process ownership) от 2026-09-05 нашло семь дефектов; все правятся одним проходом в одном worktree.

**Что сделать.**
1. probeOwnership берёт shared lock (LOCK_SH / LockFileEx без exclusive), а не эксклюзивный: иначе листинг в процессе A даёт ложный ErrBusy захвату в процессе B, и -c молча пропускает свежую сессию (internal/session/ownership.go:73-91, ownership_unix.go, ownership_windows.go). Build-тег unix-файла привести к репозиторному `!windows`.
2. ListSessions не рушится от ошибки пробы одной сессии: ошибка пробы трактуется как «не активна» с пропуском (internal/session/load.go:52-55).
3. Регрессия фокуса плана: view.go:932-935 сравнивает App.Focused() с View, а корень теперь Editor; после Alt+P и клика по нефокусируемому виджету стрелки уходят в композер. Освобождать planFocus только когда реальный фокус у текстового виджета.
4. Editor.Close закрывает View параллельно под одним контекстом и ждёт всех; View.Close проверяет closeDone неблокирующе до select с ctx (editor.go:151-158, lifecycle.go:249-253).
5. Editor.AcceptInterrupt учитывает фоновые View: при бегущей работе в любой сессии двойной Ctrl+C не завершает процесс, а показывает тост с номером сессии.
6. Настройки: один harnesssettings.Manager на процесс (cmd/session_ui.go:42), применение снапшота рассылается во все View (SetTasksAccess, compaction, notifier), CAS-конфликт покрывает все записываемые ключи.
7. Тесты cmd/session_ui_ownership_test.go и internal/tui/editor/shell_test.go зависят от системного буфера обмена (pane.go:752 pasteImage перед текстом): пробросить seam readClipboard в сборки sessions/editor/cmd и стабить его в тестах.
8. Мелочь по пути: Bus.Drain снимает wake-токен под мьютексом (bus.go:107-121); имя новой сессии из монотонного счётчика реестра, а не Len()+1 (cmd/main.go:194).

**Границы.** Не трогать recoverStale (job-recovery-process-ownership), не добавлять /close и панель (multisession-lifecycle-restore).

**Done (2026-09-05).** Коммит 39a73f3, merge 7024b9a в main. Все восемь пунктов закрыты: shared-lock проба + retry захвата 1/2/4/8 мс, ListSessions терпит ошибку пробы; фокус плана сравнивается с App.Root(); Editor.Close закрывает View параллельно, awaitDone неблокирующий; RefuseExit заменяет тост «Press Ctrl+C again» предупреждением о фоновых сессиях и разоружает выход; один harnesssettings.Manager на процесс с Attach/broadcast, миграцией планов во все сессии с откатом и CAS-токеном по всем управляемым секциям; seam SetClipboardReader в composer/View; Bus.Drain и монотонный счётчик имён. Ворота: go test -race по 8 изменённым пакетам, GOOS=windows vet и GOOS=freebsd build internal/session, golangci-lint по изменённым пакетам — 0 issues (6 pre-existing usetesting в нетронутых файлах не трогал). Живой smoke с двумя терминалами не выполнялся. Соседняя задача shell-draft-test-host-clipboard (другая сессия) покрыта тем же seam, закрыть её должна её ветка.

## Acceptance Criteria

- Проба владения не мешает конкурентному захвату: тест с горячей пробой и захватом в цикле не даёт ложного ErrBusy; -c при листинге в соседнем процессе продолжает свежую сессию
- ListSessions возвращает список при ошибке пробы одной сессии
- После Alt+P и клика по нефокусируемому виджету стрелки продолжают ходить по плану (тест)
- Editor.Close закрывает N View параллельно; успешное закрытие при истёкшем ctx не возвращает ошибку дедлайна (тест)
- Двойной Ctrl+C при бегущей фоновой сессии не завершает процесс (тест)
- Применение настроек в одной сессии доходит до остальных View; параллельный черновик другой сессии получает ErrConflict вместо тихого отката (тест)
- Тесты sessions/editor/cmd не читают системный буфер обмена; go test ./cmd ./internal/tui/editor зелёные при картинке в буфере
- Scoped go test -race по изменённым пакетам и один golangci-lint по ним зелёные

## Verification Plan

1. go test -race ./internal/session ./internal/tui/sessions ./internal/tui/editor ./internal/tui/controller ./internal/harnesssettings ./cmd в worktree
2. GOOS=windows go vet ./internal/session; GOOS=freebsd go build ./internal/session
3. golangci-lint run по изменённым пакетам один раз перед коммитом
4. Живой smoke: два терминала, в одном /sessions по кругу, в другом cozyphi -c продолжает последнюю сессию

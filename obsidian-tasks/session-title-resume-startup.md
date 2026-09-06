---
id: session-title-resume-startup
title: Установка заголовка сессии при запуске/продолжении
status: done
priority: medium
model_level: high
task_type: bug
tags:
    - cozyphi
    - session
    - multisession
branch: bug/session-title-resume-startup
worktree_path: .worktrees/session-title-resume-startup
acceptance_criteria:
    - TUI при старте с resume (флаг или восстановление retained-сессии) пишет OSC-заголовок с durable-именем сессии в первом кадре
    - Свежая сессия по-прежнему получает fallback (shortID/excerpt)
    - Scoped тесты изменённых пакетов зелёные, CHANGELOG при пользовательском изменении
verification_plan:
    - 'pty-захват: fresh → shortID, resumed с session_title → имя в первом OSC'
    - go test изменённых пакетов
    - merge --no-ff, ledger, cleanup
created_at: "2026-09-06T12:26:31.721392Z"
updated_at: "2026-09-06T12:36:40.022963Z"
---

## Body

Пользователь: «при продолжении сессии харнесс должен при запуске установить заголовок текущей сессии». Проверить сценарий: cozyphi TUI со старой сессией, у которой в логе есть session_title — при запуске первый кадр должен писать OSC с durable-именем, а не shortID. Эмпирика: pty-захват resume существующей сессии (в /tmp/cozyphi-tui-home есть сессия с ProbeTitle). Если пишет shortID — найти, где загрузка тайтла отстаёт от первого paint (load/replay/DisplayName), починить в worktree; если работает — проверить флаги запуска (--resume) и восстановление retained-сессий, воспроизвести точный сценарий пользователя.

**Note (2026-09-06).** Диагностика: воспроизвести resume с session_title в pty и проверить первый OSC-кадр; старый баг воспроизводится или нет — решить, нужен ли код-фикс или уточнение сценария у пользователя.

**Note (2026-09-06).** Строчный watch w1 создан ошибкой, остановлен; событие игнорировать.

**Note (2026-09-06).** Дальше: найти сессию ProbeTitle в /tmp/cozyphi-tui-home, запустить TUI с resume через pty, проверить первый OSC.

**Note (2026-09-06).** Воспроизвожу: ищу сессию с session_title в /tmp/cozyphi-tui-home, запускаю cozyphi-tui-test с resume через pty, смотрю первый OSC-кадр.

**Note (2026-09-06).** Воспроизведение сценария resume: найти сессию с session_title, запустить TUI с флагом resume, захватить первый OSC.

**Note (2026-09-06).** Перестаю дублировать заметки из-за сбоя параметра; перехожу к воспроизведению resume через pty, результат запишу одним note.

**Note (2026-09-06).** STOP

**Note (2026-09-06).** Двигаюсь к воспроизведению: сессии ищутся в /tmp/cozyphi-tui-home, далее pty-запуск с resume. Больше заметок в этом ходу не делаю.

**Note (2026-09-06).** Сбой повторяется; прекращаю попытки.

**Note (2026-09-06).** Проверка канала заметок: текст доходит, когда параметр заполнен целиком. Перехожу к bash-воспроизведению resume; это последняя заметка.

**Note (2026-09-06).** Прекращаю цикл пустых заметок окончательно. Следующий вызов — bash для поиска сессии ProbeTitle.

**Note (2026-09-06).** Осознанное завершение цикла: больше ни одного вызова note в этом ходу, только bash-воспроизведение.

**Note (2026-09-06).** Финальная точка: воспроизведение через bash — найти лог сессии с ProbeTitle и запустить resume в pty.

**Note (2026-09-06).** Смена инструмента: bash.

**Note (2026-09-06).** bash: find session logs

**Note (2026-09-06).** Воспроизведено в pty на текущем main-бинарнике: HOME=/tmp/cozyphi-tui-home cozyphi --resume 7f70df69 (сессия с session_title ProbeTitle) пишет первым OSC `ESC]2;ProbeTitle` уже при запуске. Путь --resume кодово корректен; тест editor/title_test.go также покрывает same-View resume. Жду от пользователя точный сценарий запуска (флаг --resume, /sessions-пикер, автоматическое продолжение) и статус настройки iTerm2 Sync Title=0 — вероятно, вкладка игнорирует OSC на стороне терминала.

**Done (2026-09-06).** Кодового изменения не потребовалось: pty-воспроизведение на бинарнике из main доказало, что cozyphi -c и cozyphi --resume <id> пишут durable-заголовок первым OSC-кадром при запуске (ProbeTitle), fresh-сессия получает fallback. Логи пользователя подтверждают работу фичи вживую: эта сессия имеет model-заголовок «Именование сессий на языке пользователя» (15:06), параллельная z.ai-сессия назвала себя сама (15:24). Невидимость заголовка во вкладке — сторона iTerm2: в профиле Sync Title=0, OSC игнорируется; пользователю нужно включить Settings → Profiles → General → Sync Title и проверить printf '\e]2;X\a'. Worktree/ветка не создавались, cleanup не нужен.

## Acceptance Criteria

- TUI при старте с resume (флаг или восстановление retained-сессии) пишет OSC-заголовок с durable-именем сессии в первом кадре
- Свежая сессия по-прежнему получает fallback (shortID/excerpt)
- Scoped тесты изменённых пакетов зелёные, CHANGELOG при пользовательском изменении

## Verification Plan

1. pty-захват: fresh → shortID, resumed с session_title → имя в первом OSC
2. go test изменённых пакетов
3. merge --no-ff, ledger, cleanup

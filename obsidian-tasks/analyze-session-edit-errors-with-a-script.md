---
id: analyze-session-edit-errors-with-a-script
title: analyze session edit errors with a script
status: done
priority: medium
task_type: feature
branch: feature/analyze-session-edit-errors-with-a-script
worktree_path: .worktrees/analyze-session-edit-errors-with-a-script
acceptance_criteria:
    - python3 scripts/analyze_edit_errors.py --root ~/.cozyphi проходит весь корпус без падений
    - 'отчёт печатает: правок всего, доля ошибок, разбивка по классам, цепочки ретраев, примеры, долю неклассифицированных'
    - классы сверены вручную на выборке реальных сессий
    - скрипт на python stdlib без зависимостей
verification_plan:
    - python3 scripts/analyze_edit_errors.py --root ~/.cozyphi
    - ручная сверка нескольких классифицированных сэмплов с реальными сессиями
created_at: "2026-09-04T21:47:30.697577Z"
updated_at: "2026-09-04T22:03:08.236977Z"
---

## Body

**What:** scripts/analyze_edit_errors.py — скрипт на python stdlib, который парсит транскрипты cozyphi (~/.cozyphi/session/* и ~/.cozyphi/jobs/*/session/*.jsonl), извлекает вызовы edit/write/read(mode=edit) и их role=tool результаты, классифицирует ошибки (нет capability, нет hash, TAG mismatch, устаревшие LINE#HASH, out of bounds, overlap, неверный формат ссылки, pasted block, план-гейт/пермиссии, write вне workspace) и печатает сводку: частоты классов, цепочки ретраев по файлу, примеры.

**Why:** пользователь требует искать типовые ошибки модели в правках файлов скриптом, не грепом; результат — материал для правок промптов/тул-описаний/механики edit (тезис: ошибка модели = недоработка харнесса).

**Done (2026-09-05).** Сделано: scripts/analyze_edit_errors.py (только stdlib) прогнан по корпусу ~/.cozyphi — 227 транскриптов (71 сессия + 156 job), 57751 записей, 0 неклассифицированных из 547 ошибок. Итог: edit 500/2905 ошибок (17.2%), write 35/430 (8.1%), read(mode=edit) 12/873, read view 233/9028 (fs-разведка, норма). Классы: stale_anchors 264 (48%), no_capability 117 (21%), plan_gate 75 (14%), tag_mismatch 56 (10%), хвост — withheld_retry/fs_error/parse_args и пр. Цепочки: 1011, слепых ретраев 385; якорные ошибки single=247/multi=193. Доля ошибок edit по датам флет 14–20%. Первопричины и рекомендации доложены пользователю 2026-09-05: fuzzy re-anchoring в writetool, capability от write на записанный путь, валидные step id в ошибках план-гейта. Коммит feature/analyze-session-edit-errors-with-a-script слит --no-ff в main (106bed9), отчёт сохранён в /tmp/edit_errors_report.txt. Санитизация: в отчёт не попадают tool args и содержимое правок.

## Acceptance Criteria

- python3 scripts/analyze_edit_errors.py --root ~/.cozyphi проходит весь корпус без падений
- отчёт печатает: правок всего, доля ошибок, разбивка по классам, цепочки ретраев, примеры, долю неклассифицированных
- классы сверены вручную на выборке реальных сессий
- скрипт на python stdlib без зависимостей

## Verification Plan

1. python3 scripts/analyze_edit_errors.py --root ~/.cozyphi
2. ручная сверка нескольких классифицированных сэмплов с реальными сессиями

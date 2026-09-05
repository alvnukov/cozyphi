---
id: session-process-ownership
title: Prevent concurrent session ownership and skip active sessions on continue
status: done
priority: high
model_level: high
task_type: bug
tags:
    - sessions
    - concurrency
branch: bug/session-process-ownership
worktree_path: .worktrees/session-process-ownership
acceptance_criteria:
    - -c продолжает последнюю незанятую сессию, пропуская активные.
    - sessions list явно показывает активные сессии.
    - Открытие занятой сессии по ID отклоняется до изменения состояния; конкурентное открытие допускает одного владельца.
    - Выход и авария владельца освобождают сессию.
verification_plan:
    - Регрессионные тесты открытия и конкурентного захвата на изолированных сессиях.
    - Тесты -c и маркировки sessions list.
    - Проверки освобождения после Close и завершения процесса; scoped tests/race/lint.
created_at: "2026-09-05T17:22:55.579201Z"
updated_at: "2026-09-05T19:00:00.011701Z"
---

## Body

Пользователь воспроизвёл вход в сессию, работающую в соседнем терминале. Нужна межпроцессная защита на весь срок владения, а не только на время генерации. -c должен выбирать последнюю свободную сессию; список должен обозначать активные. Не затрагивать действующие сессии при проверках. Отдельно от multisession-registry (реестр View внутри процесса).

**Note (2026-09-05).** Ownership, atomic continue, active listing and lifecycle cleanup implemented in task worktree; commit 7936f7c. Independent standards/spec review found switch/watch admission race; deterministic regression reproduced it, fix and repeat review passed. Session subprocess close/crash/kill/contention and scoped race tests passed; Linux/Windows/macOS CGO=0 storage test cross-builds passed. Single scoped lint found two intentional-path gosec diagnostics and one test switch suggestion, addressed without rerunning lint. Integrating concurrent main retained-view changes in worktree before final verification and merge.

**Done (2026-09-05).** Implemented in 7936f7c, integrated concurrent retained-view changes in ff5f8d9, merged into main with --no-ff (fix: merge persistent session process ownership). Stable OS sidecar locks cover idle, atomic continue skips busy, listings mark active, explicit busy opens reject before mutations. Close/crash/kill/contention isolated regressions passed; CGO=0 session builds passed Linux/Windows/macOS. Review-discovered switch/watch and local-shell cleanup races fixed, including direct controller/runtime disposal. Final targeted race tests passed in controller, sessions and cmd; final Standards/Spec review has no blockers. Scoped lint ran once: three diagnostics addressed, no repeat lint per user constraint. CHANGELOG updated; no push, live sessions and unrelated main changes untouched.

## Acceptance Criteria

- -c продолжает последнюю незанятую сессию, пропуская активные.
- sessions list явно показывает активные сессии.
- Открытие занятой сессии по ID отклоняется до изменения состояния; конкурентное открытие допускает одного владельца.
- Выход и авария владельца освобождают сессию.

## Verification Plan

1. Регрессионные тесты открытия и конкурентного захвата на изолированных сессиях.
2. Тесты -c и маркировки sessions list.
3. Проверки освобождения после Close и завершения процесса; scoped tests/race/lint.

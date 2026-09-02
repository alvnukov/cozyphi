---
id: fix-xui-loop-stop-hang
title: 'xui: Ctrl+C не выводил из TUI — Loop.Stop виснет на wg.Wait (Interrupt no-op на darwin)'
status: done
priority: high
task_type: bug
parent_id: cozyphi-enterprise-code-review
acceptance_criteria:
    - Ctrl+C завершает процесс с кодом 0 за <1с
    - ввод не деградирует (печать/стрелки/backspace)
verification_plan:
    - 'pty-стенд: exit 0.16s, marker-хук сработал'
    - ручная проверка в реальном терминале
created_at: "2026-08-23T15:57:46.944433Z"
updated_at: "2026-08-23T15:57:46.944433Z"
---

## Acceptance Criteria

- Ctrl+C завершает процесс с кодом 0 за <1с
- ввод не деградирует (печать/стрелки/backspace)

## Verification Plan

1. pty-стенд: exit 0.16s, marker-хук сработал
2. ручная проверка в реальном терминале

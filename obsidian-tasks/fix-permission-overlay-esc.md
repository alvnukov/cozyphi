---
id: fix-permission-overlay-esc
title: Esc не закрывает permission-оверлей
status: done
priority: medium
task_type: bug
parent_id: cozyphi-convenience-program
tags:
    - ui
    - permissions
acceptance_criteria:
    - Esc закрывает оверлей и отменяет запрос
    - остальные клавиши выбора работают как раньше
verification_plan:
    - ручная проверка в tmux
created_at: "2026-08-23T14:45:57.935984Z"
updated_at: "2026-08-24T01:30:39.724027Z"
---

## Body

В оверлее разрешения подсказка «Esc cancel» есть, но Esc его не закрывает. Починить обработку клавиши или убрать подсказку.

Диагноз (2026-08-23): хендлеры Esc в overlays/palette/composer корректны — событие не доходит. xui/input Parser держит одинокий 0x1b в буфере как возможное начало escape-последовательности и никогда не флашит; kitty-пуш = CSI >7u (без флага 8), так что Esc ВСЕГДА приходит голым байтом. Fix: Parser.Pending()/FlushIdle() (одинокий ESC → KeyEscape) + флаш в readLoop при простое ≥50мс.

## Acceptance Criteria

- Esc закрывает оверлей и отменяет запрос
- остальные клавиши выбора работают как раньше

## Verification Plan

1. ручная проверка в tmux

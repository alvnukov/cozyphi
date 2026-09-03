---
id: tui-resume-flags-ac-gaps
title: 'tui-resume-flags: session id не в первом кадре, ambiguous prefix без кандидатов'
status: done
priority: medium
task_type: bug
parent_id: cozyphi-enterprise-code-review
tags:
    - ux
    - cli
created_at: "2026-08-23T20:05:55.866423Z"
updated_at: "2026-08-23T20:26:05.631364Z"
---

## Body

Ревью review-deepseek-parallel-tasks, задача tui-resume-flags (c65e324), два AC-хвоста: (1) AC(4) «first frame shows resumed history + session id» — session id нигде в первом кадре не рендерится (grep по internal/tui: SessionID только в тостах /resume и /clear, session_cmds.go:105,129); (2) AC(2) «ambiguous prefix errors with candidates» — internal/session/load.go:167 отдаёт только счётчик: "ambiguous id prefix %q (%d matches)", без списка кандидатов, пользователь не видит какие префиксы уточнить. Фикс: candidates в ошибку (короткие id до N штук); session id — куда-нибудь в статусную строку/футер первого кадра (согласовать с задачей ui-turn-metadata-line, возможно одно место рендера).

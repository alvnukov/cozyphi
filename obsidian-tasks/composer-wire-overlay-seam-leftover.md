---
id: composer-wire-overlay-seam-leftover
title: 'Композер: Wire(7 параметров) и overlay-колбэк — seam не доведён до AC'
status: done
priority: medium
task_type: refactor
parent_id: cozyphi-enterprise-code-review
tags:
    - refactor
    - composer
    - seams
created_at: "2026-08-23T20:05:55.878087Z"
updated_at: "2026-08-23T20:37:58.501141Z"
---

## Body

Ревью review-deepseek-parallel-tasks, задача refactor-composer-routing-seam (87d5a33+d5a2eb2), AC(1) «Wire/bridge заменены interface с двумя adapter-ами» выполнена наполовину. commandBridge действительно заменён на Host (два адаптера: Editor + fakeHost), HookCommands собирается конструктором (AC3 ок), композер тестируется без full-app (AC2 ок, fakeBus/fakeFocus). Но ComposerPane.Wire живёт (pane.go:65): было 14 параметров, стало 7, и overlay-арбитраж ушёл сырым колбэком overlayBlocksComposer func() bool (editor.go:169 передаёт e.overlays.BlocksComposer) вместо члена seam — у колбэка один продюсер, seam не настоящий. Композер остался двухфазным («вызови Wire до использования»). Решить: либо overlay-предикат в существующий seam (Focuser/SubmitBus или новый маленький), либо честно задокументировать почему func() достаточнен; Wire-двухфазность — кандидата на слияние в конструктор newTestPane уже это делает в тестах.

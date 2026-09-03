---
id: fix-engine-midrun-swap-race
title: 'Data race: Engine mutators swap client/executor while Loop runs'
status: done
priority: high
task_type: bug
parent_id: cozyphi-enterprise-code-review
tags:
    - reliability
    - race
    - review-2026-08
created_at: "2026-08-27T16:09:20.779916Z"
updated_at: "2026-08-27T17:02:55.40526Z"
---

## Body

Engine.SetMode (engine.go:540) -> rebindTools writes engine.client (engine.go:414) and engine.executor (engine.go:477); SetPermission writes engine.executor.gate/ask (engine.go:569-572). TUI calls these from the key-event goroutine while engine.Loop runs on a streamWG.Go goroutine reading engine.client (engine.go:1260), engine.executor (engine.go:784) and gate.Check (executor.go:268). Nothing synchronizes the swap; a mid-round swap also runs tools on a new executor under an old session posture. Fix: apply mutations at tool-round boundaries via a command queue, or guard swap points and round-start reads with a mutex; route permission changes through Executor methods. Coordinate with refactor-engine-reconfigure.

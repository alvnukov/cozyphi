---
id: cleanup-dup-write0600-and-double-resolve
title: 'Чистка дублирования: write-0600 хелпер, двойной resolveWorkDir, nil-guards PaletteRoot'
status: done
priority: low
task_type: refactor
parent_id: cozyphi-enterprise-code-review
tags:
    - cleanup
    - duplication
created_at: "2026-08-23T20:05:55.890293Z"
updated_at: "2026-08-23T20:54:58.01662Z"
---

## Body

Ревью review-deepseek-parallel-tasks, дубли из пачки 1adc742..e10e947: (1) форма «открыть 0600 + Chmod» написана дважды: cmd/config.go writeOwnerOnly против инлайн-копии в internal/project/config.go SetDangerouslyAllowAll — один хелпер; (2) internal/job/manager.go Spawn резолвит workdir дважды за спавн (req.validate() и снова в Spawn); (3) internal/tui/commands/builtins.go — шесть PaletteRoot-билдеров повторяют один и тот же nil-guard на ctx.Host (set func(bool) паттерн); (4) engine.requestCompact валидирует через PrepareCompact, потом runCompaction валидирует снова на границе. Всё некритичное, но ось Architecture (deep module, consolidate) из AGENTS.md. Заодно глянуть: restoreMaskedAPIKeys (cmd/config.go:483) матчит ключи по имени модели — переименование модели при замаскированном ключе даёт ошибку валидации (fail-closed, ключ на диске цел), возможно достаточно улучшить сообщение.

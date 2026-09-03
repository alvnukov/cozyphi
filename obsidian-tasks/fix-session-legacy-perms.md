---
id: fix-session-legacy-perms
title: 'Сессии: legacy-файлы 0644 не подтягиваются до 0600 (create-mode only)'
status: done
priority: high
task_type: bug
parent_id: cozyphi-enterprise-code-review
tags:
    - security
    - permissions
created_at: "2026-08-23T20:05:55.733196Z"
updated_at: "2026-08-23T20:13:19.644384Z"
---

## Body

Ревью review-deepseek-parallel-tasks, задача fix-config-secret-perms (e10e947), AC «config.yaml/.bak/сессии 0600» выполнена частично. internal/session/manager.go:233 (flushAllEntries, O_TRUNC|O_CREATE) и :242 (appendFile, O_APPEND) передают 0600 только как create-mode — уже существующий файл с 0644 остаётся world-readable и по нему продолжают аппендиться секретные транскрипты. В том же коммите конфиг-путь чинится правильно: cmd/config.go writeOwnerOnly и internal/project/config.go делают Chmod после записи — сессионный путь должен так же. Фикс: chmod 0600 после открытия (или OpenFile + явный Chmod), тест на legacy-0644 файле.

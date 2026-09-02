---
id: fix-write-tool-resets-file-mode
title: write сбрасывает права существующего файла на 0644
status: done
priority: medium
task_type: bug
parent_id: fix-write-path-symlink-toctou
tags:
    - writetool
    - regression
    - review-2026-09
acceptance_criteria:
    - write существующего файла сохраняет его права
    - write нового файла создаёт его с 0644
verification_plan:
    - 'Тест в writetool: файл 0755, write, права остались 0755'
    - go test ./internal/tools/writetool/
created_at: "2026-09-02T16:51:43.321925Z"
updated_at: "2026-09-02T19:32:25.074132Z"
---

## Body

**Проблема.** После перехода на atomicfile `runWrite` (`internal/tools/writetool/write.go:80`) всегда пишет с `0o644`: staging-файл получает `Chmod(mode)` и переименовывается поверх цели. Раньше `os.WriteFile` сохранял права существующего файла. Путь edit права сохраняет через `Lstat` (`internal/tools/writetool/hashline.go:241`), write нет.

**Сценарий.** `write` в существующий `scripts/deploy.sh` с 0755 оставляет файл 0644, exec-бит потерян.

**Как чинить.** Как в hashline: `Lstat` цели, у существующего обычного файла брать `Mode().Perm()`, для нового файла 0644. Регрессия ветки bug/fix-write-path-symlink-toctou, найдена ревью правок после v0.19.0.

## Acceptance Criteria

- write существующего файла сохраняет его права
- write нового файла создаёт его с 0644

## Verification Plan

1. Тест в writetool: файл 0755, write, права остались 0755
2. go test ./internal/tools/writetool/

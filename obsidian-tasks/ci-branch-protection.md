---
id: ci-branch-protection
title: Включить branch protection на main (ручные настройки GitHub)
status: done
priority: medium
tags:
    - ci
    - github
    - manual
branch: task/ci-branch-protection
worktree_path: .worktrees/ci-branch-protection
created_at: "2026-08-30T20:52:51.840051Z"
updated_at: "2026-09-05T17:06:25.553085Z"
---

## Body

Файлами не делается: включить protection на main форка alvnukov/cozyphi — required status checks (lint, fmt-check, test, coverage), запрет force push. Действие за владельцем репо.

**Done (2026-09-05).** Branch protection включён и проверен на main: PR-only, 1 approve, stale dismissal, last-push approval, strict required checks, signed commits, linear history, conversation resolution, enforce_admins, запрет force-push/deletion. Прямой push отклонён. Security hardening приземлён через PR #2, SCORECARD_TOKEN и badge — через независимо approved PR #3 (merge ea7945bc). Свежий Scorecard run 33979822482 успешен: Branch-Protection теперь читается и равен 8; aggregate 7.5. До 10 Scorecard требует 2 approvals и CODEOWNERS, что выходит за зафиксированное требование задачи в 1 approve.

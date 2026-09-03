---
id: sidebar-duplicates-token-status-line-on-every-render
title: Sidebar duplicates token status line on every render
status: done
priority: high
tags:
    - issue
    - feedback
created_at: "2026-08-24T16:26:16.366386Z"
updated_at: "2026-08-24T16:36:57.513259Z"
---

## Body

User reports that the token status row in the right sidebar is appended again on every render pass. Diagnose at the render/state seam, add a regression test, and ensure repeated rendering is idempotent.

source_repo_path: /Users/zol/src/cozyphi

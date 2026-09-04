---
id: make-install-wrong-gobin
title: make install targets ~/go/bin while the live binary is ~/bin/phi
status: todo
priority: medium
task_type: bug
tags:
    - makefile
    - install
    - dx
created_at: "2026-08-23T17:51:52.37343Z"
updated_at: "2026-09-04T17:50:24.065035Z"
---

## Body

Makefile install: GOBIN ?= $(shell go env GOBIN) with GOPATH/bin fallback. On this machine GOBIN is empty and ~/go/bin is not on PATH (no phi there). The real phi lives in /Users/zol/bin/phi, so `make install` silently installs into a directory nothing reads, and the live binary only changes when someone hand-runs `CGO_ENABLED=0 go build -ldflags='-s -w' -o /Users/zol/bin/phi ./cmd`. This is exactly how a stale-binary incident happened twice (phi -c "unknown command" although main carried the fix). Fix: default GOBIN to /Users/zol/bin (or derive from `command -v phi`), document the canonical install in README/Makefile.

**Note (2026-09-04).** 2026-09-04, аудит имён: посылка задачи устарела — живой бинарник теперь ~/bin/cozyphi (Makefile: BINARY ?= cozyphi). Старый ~/bin/phi остаётся в ~/bin и есть та самая stale-ловушка (запуск phi даёт древний билд). Суть бага актуальна: make install пишет в ~/go/bin, который не на PATH. Фикс прежний: GOBIN по умолчанию в ~/bin или command -v cozyphi.

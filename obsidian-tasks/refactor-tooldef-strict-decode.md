---
id: refactor-tooldef-strict-decode
title: Strict arg decoding hand-rolled in five tool packages; move behind tooldef
status: done
priority: low
task_type: refactor
parent_id: cozyphi-enterprise-code-review
tags:
    - duplication
    - tools
    - review-2026-08
    - sector:tail
created_at: "2026-08-27T16:09:20.802361Z"
updated_at: "2026-08-28T12:13:12.344468Z"
---

## Body

Byte-identical decodeStrict in plantool.go:183 and questiontool.go:164; DisallowUnknownFields+plan_step tolerance again in watchtool/watch.go:211, memorytool/memory.go:120, lsptool/lsp.go:147; the edit file_path alias lives in permission/extract.go:50-63 AND writetool/hashline.go:249-257. One decode convention change edits five packages. Fix: a tooldef strict-decode helper and one shared path-alias resolver.

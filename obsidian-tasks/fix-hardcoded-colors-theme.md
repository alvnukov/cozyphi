---
id: fix-hardcoded-colors-theme
title: Hardcoded colors bypass Theme (mention picker selection, transcript block highlight)
status: done
priority: medium
task_type: bug
parent_id: cozyphi-enterprise-code-review
tags:
    - tui
    - theme
    - review-2026-08
    - sector:tui-shell
created_at: "2026-08-27T16:09:20.892506Z"
updated_at: "2026-08-28T11:49:08.881778Z"
---

## Body

mention/picker.go:267,299-300 (fixed selBg RGB + light-blue fg) and transcript/message_list.go:313 (fixed highlight bg) hardcode colors while command_palette.go:483-493 correctly uses th.SelectionBg/SelectionFg. Theme switches leave these stale. Add PickerSelection*/BlockHighlight theme roles and use them.

---
id: fix-thinking-block-collapsed-by-default
title: 'Thinking blocks: collapsed by default, spinner while streaming, done-state label'
status: done
task_type: bug
created_at: "2026-08-24T13:03:40.889342Z"
updated_at: "2026-08-24T13:23:14.748397Z"
---

## Body

User complaint: thinking blocks render expanded by default and keep the perpetual "Thinking" label after the model finished reasoning. Wanted: collapsed by default, animated spinner while streaming (header only), expand on press, done-state label.

Seams (pre-agreed):
- engine streamTurn stamps thinkStart (first reasoning delta) / thinkEnd (first text delta or Done); emitMessage carries thinkDur onto session.Message.ThinkingDuration.
- session.Item.ThinkingDuration; coalesceThinking sums adjacent segments; projectAssistant copies from Message.
- mapper widgetFor: Expanded: exp (drop forced || it.Streaming); pass Duration; patchItem patches it dirty.
- block.ThinkingBlock.Duration; label "Thinking" while streaming, "Thought for <opencode-style duration>" when done, interrupted unchanged. formatTurnDuration moves to components.FormatDuration (single formatter, two consumers).

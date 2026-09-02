---
id: opencode-message-layout
title: Transcript message layout ported from opencode (panel, indents, turn meta row)
status: done
created_at: "2026-08-24T06:56:13.607541Z"
updated_at: "2026-08-24T07:16:57.263719Z"
---

## Body

Port the opencode transcript layout (ground truth: ~/src/opencode/packages/tui/src/routes/session/index.tsx, UserMessage/AssistantMessage/TextPart/ReasoningPart).

Slices:
1. Theme chrome slots: Secondary (agent identity color: user bar + turn marker; opencode secondary #5c9cf5 dark / #7b5bb6 light; legacy = Accent) and BackgroundPanel (user panel bg; opencode backgroundPanel #141414 dark / #fafafa light; legacy = default bg).
2. MessageList default side padding 1 → 2 (opencode paddingLeft/Right=2).
3. UserBlock panel: ┃ bar in Secondary at col 0, BackgroundPanel fill on cols 1..w-1, padding top/bottom 1, text at x=3 wrapping to w-3 (opencode UserMessage).
4. Assistant-family left indent 3 (assistant, thinking, tool, bash, agent): new PaintRichLinesAt helper, blocks render width w-3 at x=3 (opencode paddingLeft=3).
5. Turn meta row: formatTurnMeta returns (label, tail); AssistantBlock renders "▣ " Secondary + label Foreground + " · tail" Muted (opencode AssistantMessage footer).
6. CompactionBlock: centered "── Compaction ──" rule in Border color instead of italic word (opencode compaction box).

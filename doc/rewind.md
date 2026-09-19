# Rewind

A rewind takes the conversation back to a point you choose. Everything after
that point leaves the model's context and leaves the feed. Nothing is deleted:
the session file keeps every message it ever had, and one move can be undone.

Use it when a turn went the wrong way. Use it when a long tool round filled the
window with material the next question does not need. Use it when a prompt was
worded badly and you would rather send it again than explain it twice.

## First rewind

1. Find the message you want to go back to. Every prompt you sent carries a
   `rewind` button at the right edge of its row, and so does every reply that
   finished a turn.
2. Click it. The rows after that point disappear. A notice says where the
   context now ends.
3. If you rewound to a prompt, its text is waiting in the composer. Edit it and
   send it again.
4. Changed your mind? Run `/rewind back`. The feed and the context return to
   where they were, and the composer gives back the prompt it was handed. A
   draft you had typed before the rewind, or edited after it, stays where it
   is.

The keyboard does the same thing. `/rewind <entry id>` cuts at that message, and
`/rewind back` undoes the last cut. Type `/rewind ` and press Tab for the list
of places you can cut at. It runs newest first, with a line of the message
behind each one.

## Where a cut is allowed

Two kinds of message are a turn boundary, and only those.

- **A prompt you sent.** The cut lands before it. The prompt leaves the context
  and its text comes back to the composer.
- **The reply that finished a turn.** The cut lands after it, so the reply stays
  in the context.

A reply that asked for a tool did not finish its turn. Neither a tool call nor
its result is a place to cut at: the model would be handed half a round it never
ran. Anything else is refused by name, and the refusal says what a boundary is.

Background deliveries are not offered either. A watch event, a finished
background shell and a child's outcome all wear the user role. None of them is
something you typed.

A rewind is also refused while a reply or a queued prompt is running, and the
buttons are dimmed to say so. Cancelling with Esc is not enough on its own: it
stops the stream but keeps the prompts you already sent in the queue, and each
of them is about to be appended where the cursor stands. Let the queue drain,
or clear it, and ask again.

A prompt sent with skills attached carries a harness paragraph into the log.
What comes back to the composer is what you wrote, without it.

## What a rewind does to the branch

The session file is a tree. The cursor says which leaf of it is the
conversation. A rewind moves the cursor and removes nothing. The next turn is
appended to the new cursor, so it grows a second branch out of the same message.
The branch you left keeps its entries in the file.

`/rewind back` moves the cursor to wherever the last move started from. It does
so whether or not a turn has been recorded since. The undo is itself a move, so
two of them in a row land where the pair started.

Switching between branches in the interface is not available. After a rewind the
branch you left is in the log, not on the screen.

## Log format

A move is append-only metadata in the session's JSONL log:

```json
{"type":"leaf","id":"entry-id","timestamp":"2026-09-16T09:00:00Z","target":"entry-the-cursor-moved-to","from":"entry-it-stood-on"}
```

Reopening the file replays the moves in order. The last one wins, so the cursor
comes back where you left it. An empty `target` is an empty context. That is
what a rewind to the very first prompt of a session gives.

Like a title entry, a move never enters the model's context. It is never a node
in the conversation either: nothing is ever the child of a move. Older binaries
that do not recognize `leaf` cannot resume a log that has one.

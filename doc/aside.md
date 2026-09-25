# Side questions

A side question asks the model about the conversation without adding to it.
The model reads the context as it stands, answers once, and that is the end of
it: neither the question nor the answer is shown to the model again, in this
turn or any later one. The answer is kept in the session file, so it is not
lost either.

Use it to ask what was decided three turns back, what a long tool output
amounted to, or whether the plan still makes sense, without spending the
context on the answer and without steering the next turn with it.

## First side question

1. Have a conversation with at least one message in it.
2. Type `/btw` followed by the question and press Enter, or press Ctrl+T,
   click the composer lead, select `btw` in the command palette, or enter bare
   `/btw` to open the one-shot question composer. Type the question and press
   Enter. Esc leaves the mode and keeps the draft.

   ```text
   /btw what did we decide about the retry limit?
   ```

3. The answer streams into the feed as a row of its own. The title reads
   `? btw` and the question, and the answer is drawn under it. A bar down the
   left edge carries the theme's side question color, so the row does not
   pass for a reply. The composer is free again once the answer has finished.
4. Send the next prompt as usual. The model is shown the conversation as it
   was before the question, and nothing of the question or the answer.

To ask about the conversation as it stood at an earlier message, name that
message after an `@`:

```text
/btw @3f2a9c1d0b7e4a55 why was this the answer?
```

The model is then shown everything up to that message, the message included,
and nothing after it. Type `/btw @` and press Tab for the list of messages you
can ask about. It runs newest first, with a line of each message behind its id.
The title of the row then ends with `re:` and the same line, so the answer is
read against the message it is about.

The context browser (`/context`) asks too. Select the row of a prompt or a
reply and press `b`, or open the `.` menu and pick `Ask btw about this
message`. The browser closes and the composer is in btw mode with that
message as the anchor, as if `/btw @<id>` had been typed. A summary row and a
tool row are not messages, and `b` there says so in the footer; so does a
turn still running.

## The row in the feed

A side question row is open when it appears. A click on the title folds the
answer away, and another click brings it back. Pointing at the title lights it
up, and a moment later a tooltip says whether a click folds or unfolds. The
fold stays as you left it while the answer streams and after it ends.

The title says `(cancelled)` after Esc and `(failed)` when the provider
returned an error, with the error under it. An answer that stopped at a tool
call ends with a note naming the tool. The row does not show which model
answered. The session file keeps the model name with the record.

The row has no `rewind`, `fork` or `btw` buttons. They act on messages of the
conversation, and a side question is not one.

A resumed session shows the question again, finished and open, after the
message the cursor stood on when it was asked. Turns sent later come after
it, so the row keeps its place among them. A question asked on a branch you
rewound away from is not shown while the cursor is elsewhere, and comes back
when you return to that branch. A question about history that a compaction has
since summarized opens the feed, above everything the compaction kept.

`/export` writes each side question under a `## Side question (btw)` heading:
the question as a quote, a `(cancelled)` or `(failed)` line when the answer did
not finish, a `re:` line when it was about an earlier message, then the
answer.

## What a question may be about

Any message of the current context: a prompt you sent, a reply, a reply that
called a tool, a tool result. A side question moves nothing, so it needs no
turn boundary the way a rewind or a fork does.

The context is the one the model is shown now, cut after the message you
named. A compaction summary and the blocks you deleted stay the way the model
sees them. A message on a branch you rewound away from is not in the current
context, and is refused by name. So is an id nothing in the session has.

A question is refused while a reply or a queued prompt is running, because the
context it would be asked about is still being written. Let the queue drain and
ask again. A question with no words in it, or a session with nothing in it yet,
is refused as well. Every refusal comes before the model is asked, and leaves
nothing in the file.

## What stays the same

A side question holds the pipeline while it is answered, the way `/compact`
does. A prompt you send meanwhile waits in the queue and goes out when the
answer is done. Esc cancels the answer, and the row says so.

Afterwards everything is where it was. The cursor has not moved, the context
has not changed, and the next turn is built exactly as it would have been.
Nothing the engine keeps between turns changes either. The stubs sent in
place of old tool results are the ones the last turn sent, so the part of the
prompt a provider has cached still matches.

Tools do not run. The request declares the same tools a turn does, because
some providers refuse a history with tool calls in it otherwise, and because a
different tool list would miss the provider's cache. The model is told not to
call them. When it calls one anyway the answer stops there, nothing is
executed, and the row ends with a note naming the tool.

The answer is plain model text. It is not treated as web content, and a side
question does not change whether the turn after it counts as having read the
web.

## What is kept

A finished answer is written to the session file together with the question,
the message it was asked about, where the cursor stood, the model and the
tokens it cost. An answer that stopped at a tool call is written without the
token count: the stream is dropped at the call, before the provider reports
usage. An answer that was cancelled or failed is not written: the file
holds only answers that were given in full. The row of a cancelled answer says
so, and the row of a failed one says why.

The feed rebuilds its side question rows from these records when a session is
resumed, and after a rewind or its undo. A cancelled or failed answer is not
among them, so its row stays only until the next such rebuild.

The `btw` button on a message opens the one-shot question composer at that
message's id. The composer lead shows `btw @<id>` until you ask or leave the
mode. Enter sends a side question, not a normal prompt; after a successful
send the composer returns to normal. If the question is refused, the draft and
anchor remain so you can retry. Voice dialog, shell commands, images and
pending skills cannot be mixed with side questions.

## Log format

A side question is append-only metadata in the session's JSONL log:

```json
{"type":"aside","id":"entry-id","parentID":null,"timestamp":"2026-09-21T09:00:00Z","leaf":"entry-the-cursor-stood-on","anchor":"entry-asked-about","question":"why was this the answer?","answer":"Because ...","model":"model-name","usage":{"completion_tokens":80,"prompt_tokens":1200,"total_tokens":1280}}
```

`skippedTool` is added when the answer stopped at a tool call. The id is the
one the row streamed under, so a row and its entry are the same answer under
the same name.

Like a title and a cursor move, a side question is never a node in the
conversation: nothing is its child, the cursor never lands on it, and no
context is ever built with it. A fork copies the branch and so leaves side
questions behind. Older binaries that do not recognize `aside` cannot resume a
log that has one.

To go back to an earlier moment rather than ask about it, see
[Rewind](rewind.md). To keep a second line of the conversation going, see
[Fork](fork.md).

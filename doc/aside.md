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

3. The answer streams into the feed as a row that starts with `btw:` and the
   question. The composer is free again once it has finished.
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
answer is done. Esc cancels the answer.

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

For now the answer is drawn as an ordinary reply marked `btw:`, and a session
you resume does not show it again: it is in the file, not in the replay. A row
of its own, drawn in the feed where it was asked, is the next step.

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

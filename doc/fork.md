# Fork

A fork copies the conversation up to a point you choose into a session of its
own, and opens it as another tab. Both sessions are yours to go on with: what
you ask in the copy is not in the original, and what you ask in the original is
not in the copy.

Use it when one question has two answers worth having. Use it when a long
conversation is about to turn into two, and you would rather not lose either
half. Use it when a prompt was worded badly and the old wording is still worth
keeping.

## First fork

1. Find the message to copy up to. Every prompt you sent carries a `fork`
   button at the right edge of its row, and so does every reply that finished
   a turn. The last reply carries one too: copying the whole conversation is a
   fork like any other.
2. Click it. A new tab opens, selected, holding the conversation up to that
   message and nothing after it.
3. If you forked at a prompt, its text is waiting in the composer of the new
   tab. Edit it and send it.
4. The tab you came from is where you left it: the same transcript, the same
   place in the conversation, the same draft in its composer. `/switch` and the
   tab keys go back and forth.

The keyboard does the same thing. `/fork <entry id>` copies up to that message,
and `/fork` with nothing after it copies the whole conversation as it stands.
Type `/fork ` and press Tab for the list of places you can copy up to. It runs
newest first, with a line of the message behind each one.

## Where a fork may be taken

The same two kinds of message a rewind may cut at, and only those.

- **A prompt you sent.** The copy stops before it. The prompt is not in the
  copy, and its text goes to the composer of the new tab.
- **The reply that finished a turn.** The copy stops after it, so the reply is
  in the copy.

A reply that asked for a tool did not finish its turn. Neither a tool call nor
its result is a place to copy up to: the copy would start life owing the model
the other half of a round. Anything else is refused by name.

Unlike a rewind, the reply the conversation currently ends at is on offer. A
rewind there would move nothing, while a copy of everything is a whole session
and a reasonable thing to ask for. It is what `/fork` without an id takes.

A stream cut in the middle leaves half a turn behind. `/fork` without an id
then copies up to the last whole turn. It does not refuse. The id it would
have to name in that refusal is one you never typed. The prompt whose answer
never arrived goes to the composer of the new tab, like any other. A session
with no whole turn in it yet has nothing to copy up to, and says so.

A fork is refused while a reply or a queued prompt is running, and the buttons
are dimmed to say so. The copy is taken from the branch as it stands, and a
turn still being written is not part of it yet. Let the queue drain, or clear
it, and ask again.

A fork with nowhere to land is refused before anything is written. A
sub-agent's screen has no tabs of its own. Twelve open tabs are all the tabs
there are. In both cases the button says so, and no copy is made.

A tab can still fail to open after the copy is written. The answer names the
new session and says to open it with `/resume <id>`. The conversation was
copied. Only the tab was not.

## What the copy is made of

The copy carries the branch as the session sees it: the messages, the tool
rounds inside them, and the compaction entries the context is built out of,
with their summaries and with the blocks you deleted still hidden. The model in
the new tab is shown exactly what the model in the old one was shown.

Entries keep the ids they have in the original. A row of the copy and the row
it was copied from are the same message under the same name, which is what
keeps replay and anything else holding an id pointing at the right message.

Three things stay behind: the session title, the plan and the history of cursor
moves. The copy is a new session with a title of its own to earn, and a rewind
in it starts from nothing to undo.

The original is only read. Its file is not written to, its cursor does not
move, and the next turn you take in it goes where it always would have.

## Log format

A fork is a new JSONL beside the one it came from, in the same session
directory. Its header says where it came from:

```json
{"type":"EntrySession","id":"new-session-id","timestamp":"2026-09-20T09-00-00","cwd":"/path/to/project","parentSession":"original-session-id","forkedFrom":"entry-the-copy-stops-at"}
```

`parentSession` names the session, `forkedFrom` names the message inside it.
Both are absent in a session nobody forked, and a log written before forks
existed has neither and opens exactly as it did.

The copied entries follow the header as they stood in the original, each with
its own id and its own parent. Nothing marks them as copies: inside the new
session they are the conversation, and the header is the only place that
remembers where it began.

`cozyphi sessions list` shows both sessions, and `/resume <id>` opens either.
Forking into another project is not available, and neither is merging two
branches back together.

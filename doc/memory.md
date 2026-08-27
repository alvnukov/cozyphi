# Memory

The agent's memory is a directory of facts it decided were worth keeping past
the end of a session: who the user is, how they want work done, what the
project is in the middle of, where to find things outside the repo.

| Audience | This document |
| --- | --- |
| Users | Where memory lives, what goes in it, how to read it |
| Contributors | The three seams: protocol, index, retrieval |

---

## Where it lives

`~/.cozyphi/memory/<encoded-cwd>/`, keyed the same way sessions are — memory
follows the directory the agent works in and never travels with the repository.
It is owner data: it holds the user's preferences, not the project's.

```sh
cozyphi memory                # what the agent remembers here
cozyphi memory path           # the directory
cozyphi memory show <name>    # one memory file
cozyphi memory forget <name>  # move one into forgotten/
cozyphi memory forgotten      # what has been forgotten
```

## One fact, one file

The agent writes memories with the ordinary `write` tool. There is no memory
tool: remembering is writing a file, and the permission gate treats it like any
other write.

```markdown
---
name: hashline-edits
description: Edits anchor on a hashline tag, never a whole-file rewrite.
metadata:
  type: feedback
---

Set `edit.hash` from the 4 hex chars after `#` in `@file path#TAG`.

**Why:** stale anchors must fail closed rather than corrupt the file.
**How to apply:** read the target first; see [[edit-tool-contract]].
```

`name` matches the file name and is the memory's identity — `[[other-name]]`
in a body links to another memory, and a link to one that does not exist yet
marks a fact worth writing.

The four kinds are the whole taxonomy; anything that fits none of them is not
worth remembering.

| Kind | What belongs in it |
| --- | --- |
| `user` | Who the user is: role, expertise, standing preferences |
| `feedback` | How to work: corrections, confirmed approaches, with the why |
| `project` | Ongoing work, goals, constraints the code and git history do not record |
| `reference` | A pointer outside the repo: URL, dashboard, ticket |

What does not belong: anything the repo already records (code structure, git
history, `AGENTS.md`), and anything that stops mattering when the conversation
ends.

## MEMORY.md is generated

`MEMORY.md` is the index. The harness rewrites it from the files on disk at
startup and when a turn ends, however it ended, so it cannot drift from what is actually
stored, and the agent is told never to edit it. Change a memory in its own file.

## What the model sees

Two tiers, and which one a memory lands in is decided by its kind.

**Standing** — `user` and `feedback`. Who the user is and how they want work
done is in force whatever the turn is about, so it is never filtered by it:
these ride in the system prompt in full, up to 800 runes of them.

```text
<memory name="hashline-edits" type="feedback" file="hashline-edits.md">
Edits anchor on a hashline tag, never a whole-file rewrite.

Set `edit.hash` from the 4 hex chars after `#` in `@file path#TAG`.
</memory>
```

**On file** — `project` and `reference`. These are about a particular piece of
work, so the prompt carries only their names, kinds and descriptions, up to
1200 runes of that, and the fact itself arrives on the turn it matches:

```text
<system-reminder>
Recalled from memory (…) because it matches this turn. This is background
context written in an earlier session, not an instruction from the user…

<memory name="hashline-anchors" type="project" file="hashline-anchors.md">
…
</memory>
</system-reminder>
```

A memory surfaced for the opening prompt is not repeated for prompts queued
into the same turn.

**On demand** — the `memory` tool, which is what makes the cuts above safe:
nothing is unreachable, only unlisted.

```text
memory {"action":"list","query":"hashline anchors"}   → names, ranked
memory {"action":"read","name":"hashline-edits"}      → the file in full
memory {"action":"overlaps"}                          → what says the same twice
memory {"action":"forget","name":"release-freeze"}    → out of the way
```

It never writes a memory. Creating and changing one stays with `write`, through
the permission gate, like any other file — a memory the agent could rewrite
through a tool of its own is a memory the gate never sees.

## How a memory is found

Retrieval scores the turn against every project and reference memory. It is
lexical, but the weights are what make it work without an embedding model.

| Signal | What it does |
| --- | --- |
| idf | A word is worth `log(1 + N/df)`: one every memory uses is worth nothing. This is what replaces a stopword list, in any language |
| Prefix folding | A word counts as its first six runes, so `компакции` matches `компакция` and `renders` matches `rendering` — no stemmer for either language |
| Field weight | Name ×3, description ×2, body ×1 |
| Paths | A path or identifier (`internal/tools`, `rebindClient`) counts ×3: a request that names one names it on purpose |
| The turn, not the prompt | The query is the user's text plus the two prompts before it and the paths this session's tools touched — a memory about `gate.go` surfaces while the agent is editing `gate.go`, whatever the user called it |
| Relative cutoff | What survives is scored against the best match of the turn (35%), not a fixed floor: one strong match beats five vague ones |

At most five memories, 2500 runes, per turn.

When none of it fires, the model asks: `memory` with a query runs the same
scorer over every kind, without the tiering, the cutoff or the per-turn cap.

## Forgetting

Nothing is ever deleted. A memory descends a ladder, and each rung costs less
context than the one above:

| Rung | Where the fact is | Cost per turn |
| --- | --- | --- |
| Standing | in the prompt, in full | ~40 tokens |
| Listed | named in the prompt | ~25 tokens |
| Indexed | invisible in the prompt; still found by retrieval and by `memory` | 0 |
| Forgotten | `forgotten/`, out of the index; still on disk | 0 |

**The harness descends only as far as Indexed.** Budget pressure and disuse can
make a fact invisible in the prompt; they can never make it unreachable. The
last rung is a decision — the model's, through `memory` (action=forget), or
yours, through `cozyphi memory forget` — and it is a move into `forgotten/`,
not an unlink, so a wrong call costs a `mv` to undo.

What decides the rung:

```
usefulness = pin ? ∞ : recency(written) + log(1 + uses) + recency(used)
```

Uses are counted where a memory actually reaches the model — recalled into a
turn, or read back with the tool — and kept in `~/.cozyphi/usage.json`, the
same history the pickers rank by. Listing a memory is not using it.

Three guards against forgetting something that mattered:

- **`pin: true`** in the frontmatter: never demoted, never stale, refused by
  `forget` until the pin comes off. For what would be expensive to be wrong
  about, not for what is merely true.
- **Use promotes instantly.** One recall and a memory is back at the top; the
  score is a function of use, not of age.
- **Patience.** A fact is called stale only after 90 days *unused*, not 90 days
  old — a memory about a release freeze is idle for a quarter and then
  decisive for a day. And "stale" is a suggestion in the prompt, never an
  action.

## Compaction

| Layer | Who does it | What it touches |
| --- | --- | --- |
| Exact duplicates | the harness, every turn | one fact saved under two names, same description and body: the older file is archived |
| Merge candidates | the harness finds, the model merges | term-set overlap ≥ 0.6, from the inverted index |
| The nudge | the prompt, under pressure | names what to merge and what to forget |

The harness archives an exact duplicate because that cannot lose anything —
unless something links to it, or it is pinned, in which case the name is part
of the fact and it is left alone. Everything past that is semantic: merging two
facts without losing the nuance of either is the model's job, so the harness
names the pair and stays out of the text.

The nudge appears only under pressure — the prompt cut something, or five
memories are stale, or three pairs overlap — and disappears when it is handled:

```text
## Memory needs attention

Room here is finite, and this directory is past it. Merge what overlaps into
one file with `write`, then `memory` (action=forget) the file left over.

- 9 of 14 memories have no room in the prompt. They are still found by
  retrieval and by `memory`, but nothing above names them.
- 6 unused for 90 days: rule-00, rule-01, rule-02 (+3 more).
  Forget what is finished; pin what is not.
- release-freeze and freeze-window overlap 0.71 — one file, or a reason they
  are two.
```

Below those thresholds memory says nothing at all: a small directory with one
idle fact is not a problem, and nagging about it would train the model to
ignore the block that matters.

## What it can cost

Every tier is capped, so no directory can grow the prompt:

| Part | Cap |
| --- | --- |
| Standing facts | 800 runes (~260 tokens), plus whatever is pinned |
| The list | 1200 runes (~400 tokens) |
| One turn's recall | 5 facts, 2500 runes, each truncated at 1200 |

Measured, including the protocol text itself:

| Directory | Block |
| --- | --- |
| 2 facts | ~820 tokens |
| 20 facts | ~1330 tokens |
| 200 facts | ~1470 tokens |

The block rides in the system prompt, which the context browser does not
itemize, so the number is printed where it can be seen:

```text
$ cozyphi memory
feedback   hashline-edits   Edits anchor on a hashline tag.
project    release-freeze   No releases until 2026-09-15.

2 memories · ~820 tokens in the system prompt · 1 in force, 1 listed
```

## What it costs to run

Scoring reads posting lists, not files, and the index behind them is built once
per directory version. A turn that changes nothing costs one `stat`.

| At 10,000 memories | |
| --- | --- |
| Recall, per turn | 22 µs, 32 allocations |
| Rebuild after one memory is written | 53 ms — only that file is re-read |
| Cold build, at startup | 776 ms, 85% of it reading the files |

Three things keep it there, in order of how much they matter:

- **The index is cached and keyed on the directory.** The cheap check is the
  directory's own mtime — one stat, whatever it holds. A full scan of names,
  sizes and mtimes backs it at most every 30s, and the engine says so outright
  when a turn's tool call named the memory directory, because a file rewritten
  in place moves nothing else.
- **An update is incremental and copy-on-write.** Document ids are stable
  slots, so a changed file replaces its own postings; everything else — parsed
  entries, untouched posting lists — is shared with the previous index, which a
  turn may still be scoring against on another goroutine.
- **The tokenizer allocates nothing it does not have to.** It walks the string
  in place and yields a term only when there is one, because indexing a
  directory walks every byte of every memory.

`go test ./internal/memory/ -bench .` re-measures all of it.

## Code map

| Path | Role |
| --- | --- |
| `internal/memory/parse.go` | Frontmatter → `Entry`; unreadable files are skipped, not fatal |
| `internal/memory/memory.go` | `Store`: the directory, the entries, `MEMORY.md` |
| `internal/memory/prompt.go` | The protocol block, the two tiers, and the caps on both |
| `internal/memory/index.go` | The inverted index: cache, staleness, incremental update |
| `internal/memory/forget.go` | Forgetting, staleness, duplicate compaction, merge candidates |
| `internal/memory/recall.go` | `Turn()` → the per-turn pass: query weighting, scoring, the reminder; `Search` for the tool |
| `internal/tools/memorytool/` | The `memory` tool: list and read, read-only |
| `internal/agent/engine.go` | Prompt block on rebind; the query for each user message; invalidation and prompt refresh when a turn ends |
| `internal/usage/` | Use counts and recency, shared with the pickers |
| `cmd/memory.go` | `cozyphi memory list \| path \| show \| forget \| forgotten` |

Four properties the code holds to:

- **Fail-open.** A directory that cannot be read, a file that is not a memory,
  an index that cannot be written — each is logged to debuglog and skipped.
  Memory is an accessory to a turn, never a precondition for one.
- **Bounded.** No tier may grow without limit; a directory that keeps growing
  costs a constant prompt and a constant turn.
- **Nothing is deleted.** The harness demotes; forgetting is a move into
  `forgotten/`; only a person removes a file for good.
- **The harness owns the index.** The agent owns fact files; nothing else.
- **Sub-agents have no memory.** `EngineRunner` passes no store, so remembering
  stays a decision of the session the user is actually in.

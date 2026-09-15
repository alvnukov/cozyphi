# Global memory: one fact, every repository

Status: approved design, 2026-09-16. Supersedes the implementation on
`feature/global-memory` (commit `eae2ec44`), which moved a fact between two
corpora. Companion ledger: `obsidian-tasks/global-memory.md`.

## Problem Statement

The user works across about fifteen repositories, in two harnesses that share
one memory format: cozyphi and Claude Code both read and write
`~/.claude/projects/<project>/memory/`, one topic file per fact, catalogued in
`MEMORY.md`.

Some facts are about a repository. Most of the ones that hurt when missing are
not. "Work in worktrees under `.worktrees/`", "gates cover only changed
packages", "user-visible text is written in the user's language", "use the LSP
tool instead of grep" — these are how the user wants to be worked with, and
they are true in every repository. Today each of them exists in exactly one
corpus, so:

- A rule learned in cozyphi is absent in the next repository, and the agent
  there repeats the mistake the user already corrected once.
- Copying a rule into a second corpus by hand creates a second file that
  nobody keeps in step. The user rewrote `commit-every-feature` three times;
  a hand copy would have been wrong twice.
- Claude Code has no global memory at all. Its only cross-project surface is
  `~/.claude/CLAUDE.md`, which is a single instruction file, not a corpus with
  kinds, retrieval, pinning and forgetting. So any design that keeps a shared
  fact outside a project corpus is invisible to half the user's tooling.

The constraint that shapes everything: **a fact is visible to Claude Code in a
repository only if a real `.md` file sits in that repository's corpus.**
Symlinks and hard links cannot substitute — cozyphi's write path reads with
`atomicfile.ReadNoFollow` (`O_NOFOLLOW`, fail-closed) and writes by rename, by
deliberate anti-TOCTOU design, so a link is either refused or replaced by a
regular file on the first edit.

## Solution

A fact can be marked global. From then on the harness keeps a real, ordinary
memory file for it in **every corpus it knows about**, plus one canonical copy
under the cozyphi home. Nothing about how memory is read changes: each session
still reads exactly one corpus — its own — and a global fact is simply present
there, indistinguishable at read time from a fact written in that repository.
Claude Code sees the same file and gets the same rule for free.

Globality is one line of frontmatter, `scope: global`. The harness reconciles
the copies at the start of a session and at the end of every turn: whichever
copy is newest wins and is copied over the older ones. That makes editing work
from anywhere — cozyphi in any repository, Claude Code in any repository, or
the user in an editor. A fact published from one repository is a fact the next
repository already has.

## User Stories

1. As a developer working in fifteen repositories, I want a rule I taught the
   agent once to apply in all of them, so that I do not correct the same
   mistake fifteen times.
2. As a developer, I want to mark an existing memory global without rewriting
   it, so that publishing a rule costs one command rather than a copy-paste.
3. As a developer, I want the marked fact to stay where it is, so that Claude
   Code in that repository keeps seeing it.
4. As a Claude Code user, I want global facts to appear in my project's
   `memory/` directory as ordinary topic files, so that a harness that knows
   nothing about cozyphi still benefits from them.
5. As a Claude Code user, I want global facts listed in my project's
   `MEMORY.md`, so that the catalog I read is not missing rows the directory
   has.
6. As a developer, I want to edit a global fact from whichever repository I
   happen to be in, so that fixing a rule does not mean finding the repository
   it was born in.
7. As a developer, I want an edit made in Claude Code to reach the other
   repositories too, so that I do not have to remember which harness a fact
   was last touched in.
8. As a developer, I want the newest edit to win, so that the reconciliation
   rule is one sentence I can hold in my head.
9. As a developer, I want a fact I write with `scope: global` already in it to
   be published without any further ceremony, so that "born global" needs no
   separate command.
10. As an agent, I want to publish a fact I just learned is universal, so that
    the next session in another repository starts with it.
11. As an agent, I want to un-publish a fact that turned out to be about one
    repository after all, so that a wrong call is cheap to reverse.
12. As a developer, I want un-publishing to leave the fact alive in the
    repository I ran it from, so that "this is local after all" does not mean
    "this is deleted".
13. As a developer, I want forgetting a global fact to forget it everywhere,
    so that a rule I retired does not keep applying in repositories I forgot
    about.
14. As a developer, I want every removal to be a move into `forgotten/`, so
    that a wrong removal costs a move back and never a retype.
15. As a developer, I want a pinned global fact to refuse to be forgotten, so
    that the existing pin promise is not weaker for global facts than for
    local ones.
16. As a developer, I want a global fact to be pinned everywhere or nowhere,
    so that the same rule does not have different force in different
    repositories.
17. As a developer, I want the harness never to overwrite a local fact that
    happens to share a name with a global one, so that publishing can never
    destroy something I wrote.
18. As a developer, I want such a name clash named in the prompt's maintenance
    block, so that a rule silently not applying in one repository is something
    I find out about.
19. As a developer, I want a one-line report when the harness has just written
    copies into other repositories, so that a write outside the repository I
    am sitting in is never silent.
20. As a developer, I want a global fact marked in `memory` (action=list) and
    in `cozyphi memory list`, so that I can tell at a glance which rules travel.
21. As a developer, I want `cozyphi memory global <name>` and
    `cozyphi memory local <name>`, so that publishing is available without
    starting a session.
22. As an agent, I want `memory` (action=global|local), so that publishing is
    available inside a turn.
23. As a developer, I want the prompt budgets, kinds, retrieval and pinning to
    behave exactly as they do today, so that adding global facts does not
    change how memory feels.
24. As a developer, I want reconciliation to cost nothing when nothing changed,
    so that fifteen corpora do not slow down the end of every turn.
25. As a developer, I want a repository I have never opened before to receive
    the global facts on its first session, so that a fresh clone is not a blank
    slate.
26. As a developer, I want the harness never to delete a copy on its own, so
    that a corpus I removed by accident cannot cascade into losing the fact
    everywhere.
27. As a developer, I want a global fact to survive the deletion of the
    repository it was written in, so that retiring a project does not retire
    its lessons.
28. As a developer working in a git worktree, I want the worktree to share its
    main checkout's corpus, so that `.worktrees/*` do not multiply into
    phantom repositories.
29. As a developer, I want the reconciliation to leave a file's content byte
    for byte as written, so that frontmatter I hand-edited is not reformatted
    by a copy.
30. As a developer, I want `cozyphi memory path` to name the canonical store
    as well as the project corpus, so that I can find the files.
31. As a sub-agent, I want to read the global facts my parent session reads, so
    that a delegated task works to the same rules.
32. As a developer, I want no new permission prompts from any of this, so that
    memory stays the harness's own business.
33. As a developer, I want the model to be unable to name another repository's
    corpus at all, so that distributing a fact is the harness's job and not a
    capability the model can aim somewhere I did not intend.
34. As a developer, I want the model's only lever to be "this fact is global",
    so that the blast radius of a wrong call is one flag rather than fifteen
    directories.

## Implementation Decisions

### The model

- **Globality is `scope: global` in the frontmatter.** Accepted flat or nested
  under `metadata:`, exactly like `type` and `pin` are today. `Entry` gains a
  `Global bool`. Any other value of `scope`, or its absence, means local.
- **A global fact is an ordinary memory file.** Same kinds, same tiers, same
  budgets, same retrieval, same `pin`, same `forgotten/`. Nothing in
  recall, scoring or the prompt block distinguishes it except one marker in
  list output.
- **One readable corpus.** A session reads its own
  `~/.claude/projects/<project>/memory/` and nothing else. There is no second
  readable corpus, no union, no cross-corpus scoring, no shared budget
  arithmetic. This is the whole reason the design is small: the copies make
  the reading trivial.
- **The canonical store is `~/.cozyphi/memory/`.** It holds a copy of every
  global fact and is not read by any session. Its job is to be the copy that
  outlives any particular repository and the seed for a corpus that has never
  seen the fact.
- **Identity is the file's base name.** Two copies of `commit-every-feature.md`
  in two corpora are the same fact. This matches how the copy is written and
  avoids a second identity axis.

### The registry of corpora

- The known corpora are every existing `memory/` directory under Claude Code's
  projects root — the directory `project.GlobalLayout` already computes for
  `MemoryDir()`. No liveness check: a corpus whose repository is gone still
  receives copies, so a re-clone finds its memory waiting.
- Worktrees need no handling: `MemoryDir()` already resolves a linked worktree
  to its main checkout's corpus.
- The registry is a parameter of the store, not a global: tests point it at a
  temporary tree.

### Reconciliation

One pass, run at `Open` and at the end of every turn:

1. List the canonical store and every registered corpus. Skip `MEMORY.md` and
   `forgotten/`.
2. Parse a file only when its modification time changed since the previous
   pass; the rest come from the previous pass's result. A cold start parses
   everything once.
3. Group the files carrying `scope: global` by base name. The newest
   modification time wins.
4. For every target directory — canonical store included:
   - no file of that name → write the winner;
   - a file of that name carrying `scope: global`, older → write the winner;
   - a file of that name **without** `scope: global` → leave it alone and
     record a clash.
5. Copies are byte-for-byte and carry the winner's modification time, so the
   next pass sees a settled set rather than a fresh edit. Without this the
   copies would leapfrog each other forever.
6. Re-render `MEMORY.md` in every directory that was written, from that
   directory's own files, using the existing renderer.
7. **Never delete and never move.** Removal is always an explicit operation.

Writes go through `atomicfile` with owner-only permissions, exactly as
`SyncIndex` writes `MEMORY.md` today.

### The model's reach

The model never addresses another repository. Its whole vocabulary is a fact
name in its own corpus, and the one thing it can say about that fact is
whether it is global. Every write outside the session's own corpus belongs to
the harness, is performed by the reconciler, and is not reachable from any
tool: the memory tool takes a name, never a path — which is the standing
reason the gate allows it at all — and no action accepts a repository, a
corpus or a directory as an argument.

So the model cannot publish a fact it does not hold, cannot reach into another
repository's corpus to change or retract one, and cannot cause a write to a
specific foreign directory. It marks; the harness distributes.

### Operations

- **Publish** (`MarkGlobal(name)`): add `scope: global` to the fact's own file
  if missing, preserving everything else, then reconcile. A corpus holding a
  local namesake is skipped and named in the result; the publish still
  succeeds everywhere else, because refusing outright would make a common name
  unpublishable forever.
- **Un-publish** (`MarkLocal(name)`): remove `scope: global` from the file in
  this corpus, and move the copy in the canonical store and in every other
  corpus into that corpus's `forgotten/`. This must happen in one pass:
  reconciliation never deletes, so a copy left behind would republish the fact
  on the next turn.
- **Forget** a global fact: move it into `forgotten/` in every corpus and in
  the canonical store. A pinned fact is refused, as today.
- **Born global**: a file written anywhere with `scope: global` — by cozyphi's
  `write`, by Claude Code, or by hand — is adopted by the next reconciliation
  and fanned out. Publishing is therefore possible from either harness.

### Surfaces

- `memory` tool: `global` and `local` join `list, read, overlaps, forget`.
  Each takes a fact name in this corpus and nothing else; the reconciler does
  the distributing. The permission gate's `MemoryDir` is an
  exemption that lifts the workspace-only rule so memory can be written at
  all — it is not an approval checkpoint — and `ActionMemory` is not in
  `isMutating`, so `forget` already runs in readonly mode. The new actions
  inherit exactly that posture and give up nothing that was being protected.
- `cozyphi memory global <name>` / `cozyphi memory local <name>`, matching the
  tool. `cozyphi memory path` prints the corpus and the canonical store.
- A global row is marked with `*` in both list surfaces.
- The prompt's "Memory needs attention" block gains, when either applies: one
  line naming what was copied and into how many repositories on the last pass,
  and one line per name clash naming the fact and the corpus that kept its own.
- `Engine.syncMemory` calls reconciliation alongside `Compact` and
  `SyncIndex`. No other engine change.

### What the branch loses

`feature/global-memory` implemented a second readable corpus and a move
between the two. Under this design those are dead weight and revert to `main`:
`memory.go`, `recall.go`, `forget.go`, `observe.go`, `prompt.go`, `index.go`;
`corpus.go` and `mark.go` are deleted; `OpenWithGlobal` and
`permission.Policy.GlobalMemoryDir` (with the gate, controller, bootstrap and
run wiring for it) go away entirely. What survives is the Claude-projects path
accessor, the `*` marker, the CLI and tool action names, and the documentation
structure. The result lands as one rewritten commit on a branch rebased onto
current `main`.

## Testing Decisions

A good test here asserts on what is on disk and on what a public call
returned. It never reaches into the reconciler's cache, never asserts on the
order of directory visits, and never pins the exact wording of a prompt line
beyond the fact and count it must name.

**The seam is the `memory` package's public API.** `Open` takes the canonical
store and the corpus registry; `Store` grows publish, un-publish and
reconcile; `Forget` fans out for a global fact. A test builds a temporary tree
with a canonical store and three or four corpora, drives the public calls, and
reads the files back. Nothing below that needs a seam: the engine gets one
line, the permission package is untouched, and `project` contributes a path.

Prior art in `internal/memory`: `TestSyncIndexWritesClaudeCompatibleCatalogAndReportsChange`,
`TestForgetMovesAMemoryOutOfTheWayRatherThanDeletingIt`,
`TestCompactArchivesAnExactDuplicate` — temporary directories, real files,
`require`/`assert`, assertions against the directory afterwards. New tests are
table-driven where they have more than one case.

Behaviours to cover:

- Publishing writes the fact into every corpus and into the canonical store,
  and the copies are byte-identical to the source.
- A copy carries the source's modification time, and a second reconciliation
  with nothing changed writes nothing at all.
- The newest copy wins, in every direction: canonical store to corpus, corpus
  to canonical store, corpus to corpus.
- A corpus holding a local file of the same name keeps it; the publish
  succeeds elsewhere; the clash is reported.
- A file that gains `scope: global` without any command is adopted and fanned
  out on the next pass.
- Un-publishing strips the key here and archives every other copy, and a
  following reconciliation does not bring the fact back.
- Forgetting a global fact archives it everywhere; a pinned one is refused.
- A corpus that has never seen the fact receives it on its first pass.
- Reconciliation never removes a file that no operation asked it to remove,
  including when the canonical store is empty or missing.
- `MEMORY.md` is re-rendered in the directories that were written and left
  alone in the others.

Surface tests, one each, in the style of the existing tool and command tests:
the `global` and `local` actions reach the store, and the `*` marker renders.

## Out of Scope

- Conflict detection. "Both copies changed since the last pass" is
  indistinguishable from "one copy changed" without persisted state, and the
  state is not worth its own file. Newest wins, and the loser is not preserved.
- Any merging of two facts. The existing overlap machinery already asks the
  model to merge; nothing here automates it.
- Deleting or pruning anything on the harness's initiative — orphaned copies,
  corpora of repositories that no longer exist, stale canonical entries.
- Per-repository opt-out, configuration knobs, or a cap on how many facts may
  be global.
- Any tool or command form that addresses a named repository, corpus or
  directory. Distribution has no user-facing target parameter, by design.
- Teaching Claude Code about the canonical store. It never reads it; it reads
  the copies.
- Changes to kinds, tiers, budgets, retrieval, scoring, pinning or usage
  accounting.
- A second readable corpus in any form. It was implemented and is being
  removed.

## Further Notes

Three earlier designs were tried and rejected, and the reasons are worth
keeping because each is the obvious first idea:

- **Move the file** into a global corpus (what `eae2ec44` does). It removes
  the fact from the directory Claude Code reads, which trades one problem for
  a worse one.
- **Pointer files** — a stub in the project corpus referring to the real fact.
  A second source of truth, and Claude Code reads the stub as the fact.
- **Symlinks or hard links.** Structurally impossible here:
  `atomicfile.ReadNoFollow` refuses to follow a leaf symlink and
  `atomicfile.WriteWith` replaces by rename, so a link is either denied or
  silently broken on the first write.

The copy-everywhere design is the only one that satisfies both constraints at
once — one place to edit, and a real file wherever a reader looks — and it
pays for that with a reconciler that must never delete.

Scale, measured 2026-09-16: fifteen corpora exist, nine hold facts, about
fifty facts in total, the largest corpus holding twenty-eight. A full cold
pass is well under a thousand `stat` calls, and a warm pass parses nothing.
